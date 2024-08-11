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

package controller

import (
	"cmp"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/hashicorp/go-cleanhttp"
	ollamaapi "github.com/ollama/ollama/api"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/eventrecorder"
)

type ModelReconciler struct {
	client         client.Client
	recorder       record.EventRecorder
	baseHTTPClient *http.Client
	fieldManager   string
}

func NewModelReconciler(cli client.Client, recorder record.EventRecorder) *ModelReconciler {
	return &ModelReconciler{
		client:         cli,
		recorder:       recorder,
		baseHTTPClient: cleanhttp.DefaultPooledClient(),
		fieldManager:   "model-controller",
	}
}

func (r *ModelReconciler) ollamaClientForModel(model *ollamav1alpha1.Model) *ollamaapi.Client {
	u := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(fmt.Sprintf("%s.%s.svc.cluster.local", model.GetName(), model.GetNamespace()), strconv.Itoa(DefaultOllamaPort)),
	}

	return ollamaapi.NewClient(u, r.baseHTTPClient)
}

func (r *ModelReconciler) apply(ctx context.Context, obj *unstructured.Unstructured, opts ...client.PatchOption) error {
	return r.client.Patch(ctx, obj, client.Apply,
		slices.Concat([]client.PatchOption{client.ForceOwnership, client.FieldOwner(r.fieldManager)}, opts)...,
	)
}

func (r *ModelReconciler) eventRecorderFor(obj runtime.Object) *eventrecorder.EventRecorder {
	return eventrecorder.New(r.recorder, obj)
}

func (r *ModelReconciler) Reconcile(ctx context.Context, model *ollamav1alpha1.Model) (result ctrl.Result, retErr error) {
	defer func() {
		model.Status.ObservedGeneration = model.GetGeneration()
		model.Status.OllamaImage = cmp.Or(model.Spec.OllamaImage, DefaultOllamaContainerImage)
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

	ollamaCli := r.ollamaClientForModel(model)

	resources, err := r.resources(model)
	if err != nil {
		return ctrl.Result{}, err
	}

	for i := range resources {
		if err := r.setControllerReference(model, resources[i]); err != nil {
			return ctrl.Result{}, err
		}
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
			Stream: ptr.To(false),
		}, func(resp ollamaapi.ProgressResponse) error {
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
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
	if err != nil {
		return "", false, err
	}
	// TODO consider not using kubectl codebase
	viewer := polymorphichelpers.StatefulSetStatusViewer{}
	msg, ready, err := viewer.Status(&unstructured.Unstructured{Object: content}, 0)
	if err != nil {
		return "", false, err
	}
	return strings.TrimSuffix(msg, "...\n"), ready, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.Model{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(
			errors.WithSilentRequeueOnConflict(
				reconcile.AsReconciler[*ollamav1alpha1.Model](mgr.GetClient(), r),
			),
		)
}

func (r *ModelReconciler) setControllerReference(model *ollamav1alpha1.Model, controlled metav1.Object) error {
	return ctrl.SetControllerReference(model, controlled, r.client.Scheme())
}

func (r *ModelReconciler) resources(model *ollamav1alpha1.Model) ([]*unstructured.Unstructured, error) {
	labels := map[string]string{
		"ollama.aerf.io/model": model.GetName(),
	}
	httpAPIPortName := "http-api"

	sts := applyappsv1.StatefulSet(model.GetName(), model.GetNamespace()).
		WithSpec(
			applyappsv1.StatefulSetSpec().
				WithSelector(
					applymetav1.LabelSelector().WithMatchLabels(labels),
				).
				WithServiceName(model.GetName()).
				WithReplicas(1).
				WithVolumeClaimTemplates(
					applycorev1.PersistentVolumeClaim(model.GetName()+"-ollama-root", model.GetNamespace()).
						WithSpec(
							applycorev1.PersistentVolumeClaimSpec().
								WithAccessModes(corev1.ReadWriteOnce).
								WithResources(
									applycorev1.VolumeResourceRequirements().
										WithRequests(
											corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("20Gi"), // TODO template this
											},
										),
								),
						),
				).WithTemplate(
				applycorev1.PodTemplateSpec().
					WithLabels(labels).
					WithSpec(applycorev1.PodSpec().
						WithContainers(
							applycorev1.Container().
								WithName("ollama").
								WithImage(cmp.Or(model.Spec.OllamaImage, DefaultOllamaContainerImage)).
								WithImagePullPolicy(corev1.PullIfNotPresent).
								WithResources(
									applycorev1.ResourceRequirements().
										WithRequests(model.Spec.Resources.Requests).
										WithLimits(model.Spec.Resources.Limits),
								).
								WithPorts(
									applycorev1.ContainerPort().
										WithName(httpAPIPortName).
										WithContainerPort(DefaultOllamaPort).
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
											WithPort(intstr.FromInt32(DefaultOllamaPort)).
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
		WithSpec(
			applycorev1.ServiceSpec().
				WithType(corev1.ServiceTypeClusterIP).
				WithPorts(
					applycorev1.ServicePort().
						WithName("http-api").
						WithTargetPort(intstr.FromString(httpAPIPortName)).
						WithPort(DefaultOllamaPort).
						WithProtocol(corev1.ProtocolTCP),
				).
				WithSelector(labels),
		)

	unstructuredSts, err := toUnstructured(sts)
	if err != nil {
		return nil, err
	}
	unstructuredSvc, err := toUnstructured(svc)
	if err != nil {
		return nil, err
	}
	return []*unstructured.Unstructured{
		unstructuredSts,
		unstructuredSvc,
	}, nil
}