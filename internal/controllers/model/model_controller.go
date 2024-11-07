/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package model

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	ollamaapi "github.com/ollama/ollama/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimachineryresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubectl/pkg/cmd/util/podcmd"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/applyconfig"
	"aerf.io/ollama-operator/internal/commonmeta"
	"aerf.io/ollama-operator/internal/defaults"
	"aerf.io/ollama-operator/internal/eventrecorder"
	"aerf.io/ollama-operator/internal/ollamaclient"
	"aerf.io/ollama-operator/internal/patches"

	"aerf.io/k8sutils"
	"aerf.io/k8sutils/utilreconcilers"
)

type Reconciler struct {
	client               client.Client
	recorder             record.EventRecorder
	baseHTTPClient       *http.Client
	ollamaClientProvider *ollamaclient.Provider
}

func (r *Reconciler) apply(ctx context.Context, obj *unstructured.Unstructured, opts ...client.PatchOption) error {
	return r.client.Patch(ctx, obj, client.Apply, append(opts, client.ForceOwnership)...)
}

func (r *Reconciler) eventRecorderFor(obj runtime.Object) *eventrecorder.EventRecorder {
	return eventrecorder.New(r.recorder, obj)
}

func (r *Reconciler) Reconcile(ctx context.Context, model *ollamav1alpha1.Model) (result ctrl.Result, retErr error) {
	defer func() {
		model.Status.ObservedGeneration = model.GetGeneration()
		model.Status.OllamaImage = cmp.Or(model.Spec.OllamaImage, defaults.OllamaImage)
		if retErr != nil {
			model.SetConditionsWithObservedGeneration(xpv1.ReconcileError(retErr))
		} else {
			model.SetConditionsWithObservedGeneration(xpv1.ReconcileSuccess())
		}

		patchErr := r.client.Status().Update(ctx, model)
		if patchErr != nil {
			retErr = errors.Join(retErr, patchErr)
		}
	}()

	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling Model", "object", model)
	recorder := r.eventRecorderFor(model)

	ollamaCli := r.ollamaClientProvider.ForModel(model)

	resources, err := Resources(model)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, res := range resources {
		log.V(1).Info("Applying object", "object", res)
		if err := r.apply(ctx, res); err != nil {
			return ctrl.Result{}, err
		}
	}

	sts := &appsv1.StatefulSet{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Namespace: model.GetNamespace(),
		Name:      model.GetName(),
	}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			model.SetConditionsWithObservedGeneration(xpv1.Creating())
			return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "failed to fetch statefulset to check its readiness")
	}

	readyMsg, ready, err := isStatefulSetReady(sts)
	if err != nil {
		model.SetConditionsWithObservedGeneration(xpv1.Unavailable())
		return ctrl.Result{}, err
	}
	if !ready {
		model.SetConditionsWithObservedGeneration(xpv1.Unavailable().WithMessage(readyMsg))
		return ctrl.Result{}, nil
	}

	modelList, err := ollamaCli.List(ctx)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to list local models")
	}

	if !slices.ContainsFunc(modelList.Models, func(resp ollamaapi.ListModelResponse) bool { return resp.Model == model.Spec.Model }) {
		// model has NOT been pulled in yet
		pullingModelCondition := xpv1.Creating().WithMessage(fmt.Sprintf("Pulling %q model", model.Spec.Model))
		cond := model.GetCondition(xpv1.TypeReady)
		if !cond.Equal(pullingModelCondition) {
			// pulling takes a while and we want to inform the user that it's happening
			model.SetConditionsWithObservedGeneration(pullingModelCondition)
			return ctrl.Result{Requeue: true}, nil
		}

		log.V(1).Info("started pulling ollama model")
		recorder.NormalEventf("PullingModel", "Pulling %q model", model.Spec.Model)
		pullResp := ollamaapi.ProgressResponse{}
		if err := ollamaCli.Pull(ctx, &ollamaapi.PullRequest{
			Model:  model.Spec.Model,
			Stream: ptr.To(true),
		}, func(resp ollamaapi.ProgressResponse) error {
			log.V(2).Info("pulling model...", "progressResponse", resp)
			pullResp = resp
			return nil
		}); err != nil {
			recorder.WarningEventf("PullingModel", "failed to pull %q model", model.Spec.Model)
			return ctrl.Result{}, errors.Wrapf(err, "failed to pull %q model", model.Spec.Model)
		}
		log.V(1).Info("pulled model", "response", pullResp)
		if pullResp.Status != "success" {
			model.SetConditionsWithObservedGeneration(xpv1.Unavailable().WithMessage("Model hasn't been pulled successfully, retrying"))
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	modelDetails, err := ollamaCli.Show(ctx, &ollamaapi.ShowRequest{Model: model.Spec.Model})
	if err != nil {
		model.SetConditionsWithObservedGeneration(xpv1.Unavailable())
		return ctrl.Result{}, errors.Wrap(err, "while fetching ollama model details")
	}
	model.Status.OllamaModelDetails = &ollamav1alpha1.OllamaModelDetails{
		ParameterSize:     modelDetails.Details.ParameterSize,
		QuantizationLevel: modelDetails.Details.QuantizationLevel,
		ParentModel:       modelDetails.Details.ParentModel,
		Format:            modelDetails.Details.Format,
		Family:            modelDetails.Details.Family,
		Families:          modelDetails.Details.Families,
	}

	model.SetConditionsWithObservedGeneration(xpv1.Available())
	return ctrl.Result{}, nil
}

func isStatefulSetReady(sts *appsv1.StatefulSet) (string, bool, error) {
	unstr, err := k8sutils.ToUnstructured(sts)
	if err != nil {
		return "", false, err
	}
	viewer := polymorphichelpers.StatefulSetStatusViewer{}
	msg, ready, err := viewer.Status(unstr, 0)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSuffix(msg, "...\n"), ready, nil
}

func newReconciler(cli client.Client, recorder record.EventRecorder, baseHTTPClient *http.Client, tp trace.TracerProvider) *Reconciler {
	return &Reconciler{
		client:               cli,
		recorder:             recorder,
		baseHTTPClient:       baseHTTPClient,
		ollamaClientProvider: ollamaclient.NewProvider(baseHTTPClient, tp.Tracer("ollama-client")),
	}
}

func SetupWithManager(mgr ctrl.Manager, baseHTTPClient *http.Client, tp trace.TracerProvider) error {
	r := newReconciler(mgr.GetClient(), mgr.GetEventRecorderFor("ollama-operator.model-controller"), baseHTTPClient, tp)
	reconciler := reconcile.AsReconciler(mgr.GetClient(), r)
	reconciler = utilreconcilers.NewWithTracingReconciler(
		reconciler,
		tp.Tracer("model-controller", trace.WithInstrumentationAttributes(attribute.Stringer("controller-gvk", ollamav1alpha1.ModelGroupVersionKind))),
	)
	reconciler = utilreconcilers.RequeueOnConflict(reconciler)

	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.Model{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(reconciler)
}

func Resources(model *ollamav1alpha1.Model) ([]*unstructured.Unstructured, error) {
	labels := commonmeta.LabelsForResource(model.GetName(), map[string]string{
		"ollama.aerf.io/model": model.GetName(),
	})
	httpAPIPortName := "http-api"
	containerName := "ollama"
	sts := applyappsv1.StatefulSet(model.GetName(), model.GetNamespace()).
		WithLabels(labels).
		WithOwnerReferences(
			applyconfig.ControllerReferenceFrom(model)).
		WithSpec(
			applyappsv1.StatefulSetSpec().
				WithSelector(applymetav1.LabelSelector().WithMatchLabels(labels)).
				WithServiceName(model.GetName()).
				WithReplicas(1). // do NOT adapt this field, the current logic only allows for replicas=1
				WithMinReadySeconds(10).
				WithVolumeClaimTemplates(
					applycorev1.PersistentVolumeClaim(model.GetName()+"-ollama-root", model.GetNamespace()).
						WithSpec(
							applycorev1.PersistentVolumeClaimSpec().
								WithAccessModes(corev1.ReadWriteOnce).
								WithResources(
									applycorev1.VolumeResourceRequirements().
										WithRequests(
											corev1.ResourceList{
												corev1.ResourceStorage: apimachineryresource.MustParse("20Gi"),
											},
										),
								),
						),
				).WithTemplate(
				applycorev1.PodTemplateSpec().
					WithLabels(labels).
					WithAnnotations(map[string]string{
						podcmd.DefaultContainerAnnotationName: containerName,
					}).
					WithSpec(applycorev1.PodSpec().
						WithContainers(
							applycorev1.Container().
								WithName(containerName).
								WithImage(cmp.Or(model.Spec.OllamaImage, defaults.OllamaImage)).
								WithImagePullPolicy(corev1.PullIfNotPresent).
								WithPorts(
									applycorev1.ContainerPort().
										WithName(httpAPIPortName).
										WithContainerPort(defaults.OllamaPort).
										WithProtocol(corev1.ProtocolTCP),
								).
								WithEnv(
									applycorev1.EnvVar().WithName("OLLAMA_KEEP_ALIVE").WithValue("-1"), // infinity
									applycorev1.EnvVar().WithName("OLLAMA_MAX_LOADED_MODELS").WithValue("1"),
									applycorev1.EnvVar().WithName("OLLAMA_DEBUG").WithValue("false"),
								).
								WithLivenessProbe(
									applycorev1.Probe().
										WithInitialDelaySeconds(10).
										WithFailureThreshold(3).
										WithPeriodSeconds(5).
										WithHTTPGet(
											applycorev1.HTTPGetAction().
												WithPort(intstr.FromString(httpAPIPortName)).
												WithPath("/"),
										),
								).
								WithReadinessProbe(applycorev1.
									Probe().
									WithInitialDelaySeconds(10).
									WithFailureThreshold(3).
									WithPeriodSeconds(5).
									WithHTTPGet(
										applycorev1.HTTPGetAction().
											WithPort(intstr.FromInt32(defaults.OllamaPort)).
											WithPath("/"),
									),
								).
								WithVolumeMounts(
									applycorev1.VolumeMount().
										WithName(model.GetName() + "-ollama-root").
										WithMountPath("/root/.ollama"),
								),
						),
					),
			),
		)

	svc := applycorev1.Service(model.GetName(), model.GetNamespace()).
		WithLabels(labels).
		WithOwnerReferences(applyconfig.ControllerReferenceFrom(model)).
		WithSpec(
			applycorev1.ServiceSpec().
				WithType(corev1.ServiceTypeClusterIP).
				WithPorts(
					applycorev1.ServicePort().
						WithName("http-api").
						WithTargetPort(intstr.FromString(httpAPIPortName)).
						WithPort(defaults.OllamaPort).
						WithProtocol(corev1.ProtocolTCP),
				).
				WithSelector(labels),
		)

	patchedSts, err := patches.Apply(sts, model.Spec.StatefulSetPatches)
	if err != nil {
		return nil, err
	}
	patchedSvc, err := patches.Apply(svc, model.Spec.ServicePatches)
	if err != nil {
		return nil, err
	}

	unstructuredSts, err := k8sutils.ToUnstructured(patchedSts)
	if err != nil {
		return nil, err
	}
	unstructuredSvc, err := k8sutils.ToUnstructured(patchedSvc)
	if err != nil {
		return nil, err
	}

	return []*unstructured.Unstructured{
		unstructuredSts,
		unstructuredSvc,
	}, nil
}
