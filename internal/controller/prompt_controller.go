package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/hashicorp/go-cleanhttp"
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
	ollamaCli := r.ollamaClientForModel(referencedModel)

	opts, err := r.getOptionsFromSpecOptions(prompt)
	if err != nil {
		return reconcile.Result{}, err
	}

	generateResp := ollamaapi.GenerateResponse{}
	err = ollamaCli.Generate(ctx, &ollamaapi.GenerateRequest{
		Model:    referencedModel.Spec.Model,
		Prompt:   prompt.Spec.Prompt,
		Suffix:   prompt.Spec.Suffix,
		System:   prompt.Spec.System,
		Template: prompt.Spec.Template,
		Context:  nil, // todo
		Stream:   ptr.To(false),
		Raw:      false,
		Images:   nil, // todo
		Options:  opts,
	}, func(resp ollamaapi.GenerateResponse) error {
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
		EvalCount:          int64(generateResp.Metrics.EvalCount),
		EvalDuration:       metav1.Duration{Duration: generateResp.Metrics.EvalDuration},
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
