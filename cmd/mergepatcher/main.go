package main

import (
	"cmp"
	_ "embed"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/yaml"

	cmnv1alpha1 "aerf.io/ollama-operator/apis/common/v1alpha1"
	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/controller"
	"aerf.io/ollama-operator/internal/patches"
)

//go:embed values.yaml
var sourceYaml []byte

func main() {
	sts := stsResource()

	doc := &Document{}
	if err := yaml.Unmarshal(sourceYaml, &doc); err != nil {
		panic(err)
	}

	patched, err := patches.Apply(sts, doc.StsMergePatch)
	if err != nil {
		panic(err)
	}

	out, err := yaml.Marshal(patched)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}

type Document struct {
	StsMergePatch *cmnv1alpha1.Patches `json:"statefulSet,omitempty"`
}

func stsResource() *applyappsv1.StatefulSetApplyConfiguration {
	model := &ollamav1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model",
			Namespace: "default",
		},
	}
	labels := map[string]string{
		"ollama.aerf.io/model": model.GetName(),
	}
	httpAPIPortName := "http-api"
	return applyappsv1.StatefulSet(model.GetName(), model.GetNamespace()).
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
								WithImage(cmp.Or(model.Spec.OllamaImage, controller.DefaultOllamaContainerImage)).
								WithImagePullPolicy(corev1.PullIfNotPresent).
								WithPorts(
									applycorev1.ContainerPort().
										WithName(httpAPIPortName).
										WithContainerPort(controller.DefaultOllamaPort).
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
											WithPort(intstr.FromInt32(controller.DefaultOllamaPort)).
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

}
