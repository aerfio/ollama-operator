package prompt

import (
	"bytes"
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	ollamaapi "github.com/ollama/ollama/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/ollamaclient"

	"aerf.io/k8sutils/k8stracing"
	"aerf.io/k8sutils/utilreconcilers"
)

type Reconciler struct {
	client               client.Client
	recorder             record.EventRecorder
	baseHTTPClient       *http.Client
	ollamaClientProvider *ollamaclient.Provider
}

func NewReconciler(cli client.Client, recorder record.EventRecorder, httpCli *http.Client, tp trace.TracerProvider) *Reconciler {
	return &Reconciler{
		client:               client.WithFieldOwner(cli, "ollama-operator.prompt-controller"),
		recorder:             recorder,
		baseHTTPClient:       httpCli,
		ollamaClientProvider: ollamaclient.NewProvider(httpCli, tp.Tracer("prompt-controller.ollama-client")),
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager, tp trace.TracerProvider) error {
	k8sCli := client.WithFieldOwner(
		client.WithFieldValidation(
			k8stracing.NewK8sClient(mgr.GetClient(), tp),
			metav1.FieldValidationStrict),
		"ollama-operator.prompt-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.Prompt{}).
		WatchesRawSource(source.Kind(mgr.GetCache(), &ollamav1alpha1.Model{}, handler.TypedEnqueueRequestsFromMapFunc[*ollamav1alpha1.Model](func(ctx context.Context, model *ollamav1alpha1.Model) []reconcile.Request {
			log := mgr.GetLogger().WithValues("controller", "prompt-controller")

			promptList := &ollamav1alpha1.PromptList{}
			if err := k8sCli.List(ctx, promptList, client.InNamespace(model.GetNamespace())); err != nil {
				log.Error(err, "unable to list prompts")
				return nil
			}

			var ctrlRequests []ctrl.Request
			for _, prompt := range promptList.Items {
				if prompt.Spec.ModelRef.Name == model.GetName() {
					ctrlRequests = append(ctrlRequests, ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: prompt.GetNamespace(),
							Name:      prompt.GetName(),
						},
					})
				}
			}
			return ctrlRequests
		}))).
		Complete(
			utilreconcilers.NewWithTracingReconciler(
				errors.WithSilentRequeueOnConflict(
					reconcile.AsReconciler[*ollamav1alpha1.Prompt](k8sCli, r),
				),
				tp.Tracer("prompt-controller", trace.WithInstrumentationAttributes(attribute.Stringer("controller-gvk", ollamav1alpha1.PromptGroupVersionKind))),
			),
		)
}

func (r *Reconciler) Reconcile(ctx context.Context, prompt *ollamav1alpha1.Prompt) (result ctrl.Result, retErr error) {
	defer func() {
		prompt.Status.ObservedGeneration = prompt.GetGeneration()
		if retErr != nil {
			prompt.SetConditionsWithObservedGeneration(xpv1.ReconcileError(retErr))
		} else {
			prompt.SetConditionsWithObservedGeneration(xpv1.ReconcileSuccess())
		}

		patchErr := r.client.Status().Update(ctx, prompt)
		if patchErr != nil {
			retErr = errors.Join(retErr, patchErr)
		}
	}()

	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling Prompt", "object", prompt)

	// TODO: invent something more robust
	// TODO rerun prompt if query changed (or forbid changes to prompt fields)
	if prompt.Status.Response != "" {
		return reconcile.Result{}, nil
	}

	referencedModel := &ollamav1alpha1.Model{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Namespace: cmp.Or(prompt.Spec.ModelRef.Namespace, prompt.GetNamespace()),
		Name:      prompt.Spec.ModelRef.Name,
	}, referencedModel); err != nil {
		if apierrors.IsNotFound(err) {
			prompt.SetConditionsWithObservedGeneration(xpv1.Unavailable().WithMessage("Referenced model does not exist"))
			log.V(1).Info("referenced model does not exist")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("failed to fetch model: %w", err)
	}

	if !referencedModel.Status.Equal(xpv1.NewConditionedStatus(xpv1.Available(), xpv1.ReconcileSuccess())) {
		prompt.SetConditionsWithObservedGeneration(xpv1.Unavailable().WithMessage("Model is not ready and synced"))
		return reconcile.Result{}, nil
	}

	waitingForResponseCond := xpv1.Creating().WithMessage("Waiting for model response")
	if !prompt.Status.GetCondition(xpv1.TypeReady).Equal(waitingForResponseCond) {
		prompt.SetConditionsWithObservedGeneration(waitingForResponseCond)
		return reconcile.Result{Requeue: true}, nil
	}

	ollamaCli := r.ollamaClientProvider.ForModel(referencedModel)

	opts, err := r.getOptionsFromSpecOptions(prompt)
	if err != nil {
		return reconcile.Result{}, err
	}

	var imageData []ollamaapi.ImageData
	if len(prompt.Spec.Images) > 0 {
		for _, image := range prompt.Spec.Images {
			extracted, err := extractRawImageData(ctx, r.client, image)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to extract image: %w", err)
			}

			imgData, err := convertImageData(extracted)
			if err != nil {
				return ctrl.Result{}, errors.WithMessagef(err, "failed to decode images")
			}
			imageData = append(imageData, imgData)
		}
	}

	var specContext []int
	if prompt.Spec.Context != "" {
		decoded, err := base64.StdEncoding.DecodeString(prompt.Spec.Context)
		if err != nil {
			return ctrl.Result{}, errors.WithMessagef(err, "failed to decode context")
		}
		promptCtx := []int{}
		if err := json.Unmarshal(decoded, &promptCtx); err != nil {
			return ctrl.Result{}, errors.WithMessagef(err, "failed to unmarshal context into []int")
		}
		specContext = promptCtx
	}

	generateResp := ollamaapi.GenerateResponse{}
	req := &ollamaapi.GenerateRequest{
		Model:    referencedModel.Spec.Model,
		Prompt:   prompt.Spec.Prompt,
		Suffix:   prompt.Spec.Suffix,
		System:   prompt.Spec.System,
		Template: prompt.Spec.Template,
		Context:  specContext,
		Stream:   ptr.To(false),
		Raw:      false,
		Images:   imageData,
		Options:  opts,
	}
	err = ollamaCli.Generate(ctx, req, func(resp ollamaapi.GenerateResponse) error {
		generateResp = resp
		return nil
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to generate prompt: %w", err)
	}

	// todo handle done, doneReason

	marshalledContext, err := json.Marshal(generateResp.Context)
	if err != nil {
		return reconcile.Result{}, err
	}

	prompt.Status.Context = base64.StdEncoding.EncodeToString(marshalledContext)
	prompt.Status.Response = strings.TrimSpace(strings.TrimRight(generateResp.Response, " \n"))
	prompt.Status.PromptResponseMeta = &ollamav1alpha1.PromptResponseMeta{
		CreatedAt: metav1.NewTime(generateResp.CreatedAt),
	}
	prompt.Status.PromptResponseMetrics = &ollamav1alpha1.PromptResponseMetrics{
		TotalDuration:      metav1.Duration{Duration: generateResp.Metrics.TotalDuration},
		LoadDuration:       metav1.Duration{Duration: generateResp.Metrics.LoadDuration},
		PromptEvalCount:    int64(generateResp.Metrics.PromptEvalCount),
		PromptEvalDuration: metav1.Duration{Duration: generateResp.Metrics.PromptEvalDuration},
		PromptEvalRate:     fmt.Sprintf("%.2f tokens/s", float64(generateResp.PromptEvalCount)/generateResp.PromptEvalDuration.Seconds()),
		EvalCount:          int64(generateResp.Metrics.EvalCount),
		EvalDuration:       metav1.Duration{Duration: generateResp.Metrics.EvalDuration},
		EvalRate:           fmt.Sprintf("%.2f tokens/s", float64(generateResp.EvalCount)/generateResp.EvalDuration.Seconds()),
	}

	prompt.SetConditionsWithObservedGeneration(xpv1.Available())

	return reconcile.Result{}, nil
}

func (r *Reconciler) getOptionsFromSpecOptions(prompt *ollamav1alpha1.Prompt) (map[string]any, error) {
	raw := prompt.Spec.Options.Raw
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any)
	return out, errors.WithMessage(json.Unmarshal(raw, &out), "failed to unmarshal options into json struct")
}

func convertImageData(data ollamav1alpha1.ImageData) (ollamaapi.ImageData, error) {
	switch data.Format {
	case ollamav1alpha1.ImageFormatNone, "":
		content, err := base64.StdEncoding.DecodeString(data.Data)
		return content, errors.Wrap(err, "failed to decode image in base64")
	case ollamav1alpha1.ImageFormatGzip:
		b64Dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(data.Data))
		gzipReader, err := gzip.NewReader(b64Dec)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()
		content, err := io.ReadAll(gzipReader)
		return content, errors.Wrap(multierr.Append(err, gzipReader.Close()), "while decompressing image encoded in gzip format")
	case ollamav1alpha1.ImageFormatZstd:
		content, err := DecodeBase64Zstd([]byte(data.Data))
		return content, errors.Wrap(err, "while decompressing image encoded in zstd")
	default:
		panic(fmt.Sprintf("Unknown image format %q, available formats: %+v. This panic should never happen, please file an issue in source repository", data.Format, ollamav1alpha1.ImageFormatAll))
	}
}

func DecodeBase64Zstd(input []byte) ([]byte, error) {
	b64Dec := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(input))
	zstdReader, err := zstd.NewReader(b64Dec)
	if err != nil {
		return nil, err
	}
	defer zstdReader.Close()
	return io.ReadAll(zstdReader)
}

func extractRawImageData(ctx context.Context, cli client.Reader, ref ollamav1alpha1.ImageSource) (ollamav1alpha1.ImageData, error) {
	if ref.Inline != nil {
		return ollamav1alpha1.ImageData{
			Format: ref.Inline.Format,
			Data:   ref.Inline.Data,
		}, nil
	}

	if ref.ConfigMapKeyRef != nil {
		cm := &corev1.ConfigMap{}
		if err := cli.Get(ctx, client.ObjectKey{
			Namespace: ref.ConfigMapKeyRef.Namespace,
			Name:      ref.ConfigMapKeyRef.Name,
		}, cm); err != nil {
			return ollamav1alpha1.ImageData{}, fmt.Errorf("failed to get cm %s/%s: %s", ref.ConfigMapKeyRef.Name, ref.ConfigMapKeyRef.Namespace, err)
		}
		data, ok := cm.Data[ref.ConfigMapKeyRef.Key]
		if !ok {
			return ollamav1alpha1.ImageData{}, fmt.Errorf("key %q not found in configmap %s", ref.ConfigMapKeyRef.Key, ref.ConfigMapKeyRef.Name)
		}
		imgData := ollamav1alpha1.ImageData{}
		return imgData, errors.Wrap(yaml.Unmarshal([]byte(data), &imgData), "failed to unmarshal image data")
	}

	secret := &corev1.Secret{}
	if err := cli.Get(ctx, client.ObjectKey{
		Namespace: ref.SecretKeyRef.Namespace,
		Name:      ref.SecretKeyRef.Name,
	}, secret); err != nil {
		return ollamav1alpha1.ImageData{}, err
	}
	data, ok := secret.Data[ref.SecretKeyRef.Key]
	if !ok {
		return ollamav1alpha1.ImageData{}, fmt.Errorf("key %q not found in secret %s", ref.SecretKeyRef.Key, ref.SecretKeyRef.Name)
	}
	imgData := ollamav1alpha1.ImageData{}
	return imgData, errors.Wrap(yaml.Unmarshal(data, &imgData), "failed to unmarshal image data")
}
