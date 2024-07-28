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
	"context"

	"github.com/fluxcd/pkg/ssa"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/api/v1alpha1"
)

const DefaultOllamaPort = 11434

type ModelReconciler struct {
	client          client.Client
	resourceManager *ssa.ResourceManager
}

func NewModelReconciler(client client.Client) *ModelReconciler {
	return &ModelReconciler{
		client: client,
		resourceManager: ssa.NewResourceManager(client, nil, ssa.Owner{
			Field: "ollama-operator",
			Group: "ollama.aerf.io",
		}),
	}
}

func (r *ModelReconciler) Reconcile(ctx context.Context, model *ollamav1alpha1.Model) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("Reconciling Model")

	resources, err := r.resources(model)
	if err != nil {
		return ctrl.Result{}, err
	}

	for i := range resources {
		resource := resources[i]
		if err := r.setControllerReference(model, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	changeSet, err := r.resourceManager.ApplyAll(ctx, resources, ssa.ApplyOptions{
		Force: true,
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	log.V(1).Info("applied resources", "changeSet", changeSet.ToMap())

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ollamav1alpha1.Model{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Complete(reconcile.AsReconciler[*ollamav1alpha1.Model](mgr.GetClient(), r))
}

func (r *ModelReconciler) setControllerReference(model *ollamav1alpha1.Model, controlled metav1.Object) error {
	return ctrl.SetControllerReference(model, controlled, r.client.Scheme())
}
func toUnstructured(obj any) (*unstructured.Unstructured, error) {
	unstr := &unstructured.Unstructured{}
	var err error
	unstr.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return unstr, nil
}

func (r *ModelReconciler) resources(model *ollamav1alpha1.Model) ([]*unstructured.Unstructured, error) {
	labels := map[string]string{
		"ollama.aerf.io/model": model.GetName(),
	}

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
												corev1.ResourceStorage: resource.MustParse("20Gi"),
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
								WithImage("ollama/ollama:latest").
								WithImagePullPolicy(corev1.PullIfNotPresent).
								WithPorts(
									applycorev1.ContainerPort().
										WithName("http-api").
										WithContainerPort(DefaultOllamaPort).
										WithProtocol(corev1.ProtocolTCP),
								).
								WithEnv(
									applycorev1.EnvVar().WithName("OLLAMA_KEEP_ALIVE").WithValue("5m"),
									applycorev1.EnvVar().WithName("OLLAMA_MAX_LOADED_MODELS").WithValue("1"),
									applycorev1.EnvVar().WithName("OLLAMA_DEBUG").WithValue("false"),
								).
								WithLivenessProbe(
									applycorev1.Probe().WithHTTPGet(
										applycorev1.HTTPGetAction().
											WithPort(intstr.FromInt32(DefaultOllamaPort)).
											WithPath("/version")),
								),
						),
					),
			),
		)

	svc := applycorev1.Service(model.GetName(), model.GetNamespace()).
		WithLabels(labels).
		WithSpec(
			applycorev1.ServiceSpec().
				WithType(corev1.ServiceTypeClusterIP).WithPorts(
				applycorev1.ServicePort().
					WithName("http-api").
					WithPort(DefaultOllamaPort).
					WithProtocol(corev1.ProtocolTCP),
			),
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
