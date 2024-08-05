package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	ollamaapi "github.com/ollama/ollama/api"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/api/v1alpha1"
)

type PromptReconciler struct {
	client         client.Client
	recorder       record.EventRecorder
	baseHTTPClient *http.Client
	fieldManager   string
}

func NewPromptReconciler(cli client.Client, recorder record.EventRecorder) *PromptReconciler {
	return &PromptReconciler{
		client:         cli,
		recorder:       recorder,
		baseHTTPClient: cleanhttp.DefaultPooledClient(),
		fieldManager:   "prompt-controller",
	}
}

func (r *PromptReconciler) ollamaClientForModel(model *ollamav1alpha1.Model) *ollamaapi.Client {
	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(fmt.Sprintf("%s.%s.svc.cluster.local", model.GetName(), model.GetNamespace()), strconv.Itoa(DefaultOllamaPort)),
	}

	return ollamaapi.NewClient(u, r.baseHTTPClient)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PromptReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.Prompt{},
			builder.WithPredicates(
				predicate.And[client.Object](
					predicate.GenerationChangedPredicate{},
					predicate.ResourceVersionChangedPredicate{},
				),
			),
		).
		Watches(&ollamav1alpha1.Model{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
			log := mgr.GetLogger().WithValues("controller", "prompt-controller")
			model := obj.(*ollamav1alpha1.Model)

			promptList := &ollamav1alpha1.PromptList{}
			if err := mgr.GetClient().List(ctx, promptList, client.InNamespace(model.GetNamespace())); err != nil {
				log.Error(err, "unable to list prompts")
				return nil
			}

			ctrlRequests := []ctrl.Request{}
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
		})).
		Complete(
			errors.WithSilentRequeueOnConflict(
				reconcile.AsReconciler[*ollamav1alpha1.Prompt](mgr.GetClient(), r),
			),
		)
}

func (r *PromptReconciler) Reconcile(ctx context.Context, prompt *ollamav1alpha1.Prompt) (result ctrl.Result, retErr error) {
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
		Namespace: prompt.Spec.ModelRef.Namespace,
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

	ollamaCli := r.ollamaClientForModel(referencedModel)

	opts, err := r.getOptionsFromSpecOptions(prompt)
	if err != nil {
		return reconcile.Result{}, err
	}

	var imageData []ollamaapi.ImageData
	if len(prompt.Spec.Images) > 0 {
		for _, image := range prompt.Spec.Images {
			imgData, err := convertImageData(image)
			if err != nil {
				return ctrl.Result{}, errors.WithMessagef(err, "failed to decode images")
			}
			imageData = append(imageData, imgData)
		}
	}

	generateResp := ollamaapi.GenerateResponse{}
	req := &ollamaapi.GenerateRequest{
		Model:    referencedModel.Spec.Model,
		Prompt:   prompt.Spec.Prompt,
		Suffix:   prompt.Spec.Suffix,
		System:   prompt.Spec.System,
		Template: prompt.Spec.Template,
		Context:  nil, // todo
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
	//jsonResp, err := json.Marshal(generateResp)
	//if err != nil {
	//	return reconcile.Result{}, err
	//}
	// fmt.Fprintf(os.Stderr, "prompt eval rate:     %.2f tokens/s\n", float64(m.PromptEvalCount)/m.PromptEvalDuration.Seconds())

	log.Info("generatedResponse", "resp", generateResp)
	// todo handle done, doneReason
	prompt.Status.Response = generateResp.Response
	prompt.Status.PromptResponseMeta = &ollamav1alpha1.PromptResponseMeta{
		Context:   intToInt64Slice(generateResp.Context),
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

func intToInt64Slice(in []int) []int64 {
	out := make([]int64, len(in))
	for i := range in {
		out[i] = int64(in[i])
	}
	return out
}

func (r *PromptReconciler) getOptionsFromSpecOptions(prompt *ollamav1alpha1.Prompt) (map[string]any, error) {
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
		return content, errors.Wrap(err, "while decompressing image encoded in gzip format")
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

//
//
//curl http://localhost:11434/api/generate -d '{
//"model": "llava",
//"prompt":"What is in this picture?",
//"suffix": "",
//"system": "",
//"template": "",
//"stream": false,
//"format": "",
//"options": null,
//"images": ["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"]
//}'
