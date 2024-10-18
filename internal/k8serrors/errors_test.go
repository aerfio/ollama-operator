package k8serrors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	applyappsv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"aerf.io/k8sutils"
)

func TestIsImmutable(t *testing.T) {
	testEnv := envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     []string{"testdata"},
	}
	restCfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testEnv.Stop())
	})
	cli, err := client.New(restCfg, client.Options{})
	require.NoError(t, err)
	t.Run("detects immutable fields via CEL validation in CRD", func(t *testing.T) {
		testObj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "aerf.io/v1",
				"kind":       "Test",
				"metadata": map[string]any{
					"name":      "test-name",
					"namespace": "default",
				},
				"spec": map[string]any{
					"foo": "bar",
				},
			},
		}
		gvk := testObj.GroupVersionKind()
		name := testObj.GetName()
		namespace := testObj.GetNamespace()
		err := cli.Create(context.Background(), testObj)
		require.NoError(t, err)

		objToUpdate := &unstructured.Unstructured{}
		objToUpdate.SetGroupVersionKind(gvk)
		err = cli.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, objToUpdate)
		require.NoError(t, err)

		err = unstructured.SetNestedField(objToUpdate.Object, "bar2", "spec", "foo")
		require.NoError(t, err)

		err = cli.Update(context.Background(), objToUpdate)
		require.Error(t, err)
		require.True(t, IsImmutable(err))
	})
	t.Run("detects immutable fields when updating built-in resources like StatefulSet", func(t *testing.T) {
		testSts := applyappsv1.StatefulSet("sts", "default").
			WithSpec(
				applyappsv1.
					StatefulSetSpec().
					WithServiceName("does-not-exist").
					WithSelector(
						applymetav1.LabelSelector().
							WithMatchLabels(map[string]string{"app": "nginx"}),
					).
					WithTemplate(
						applycorev1.PodTemplateSpec().
							WithLabels(map[string]string{"app": "nginx"}).
							WithSpec(
								applycorev1.PodSpec().
									WithContainers(
										applycorev1.Container().
											WithName("nginx").
											WithImage("nginx:latest"),
									),
							),
					).
					WithVolumeClaimTemplates(
						applycorev1.PersistentVolumeClaim("www", "default").
							WithSpec(
								applycorev1.PersistentVolumeClaimSpec().
									WithAccessModes(
										corev1.PersistentVolumeAccessMode("ReadWriteOnce"),
									).WithResources(
									applycorev1.VolumeResourceRequirements().
										WithRequests(
											corev1.ResourceList{
												corev1.ResourceStorage: resource.MustParse("10Gi"),
											},
										),
								),
							),
					),
			)
		unstr, err := k8sutils.ToUnstructured(testSts)
		require.NoError(t, err)

		err = cli.Patch(context.Background(), unstr, client.Apply, client.FieldOwner("ollama-operator"))
		require.NoError(t, err)

		testSts.Spec.VolumeClaimTemplates[0].Name = ptr.To("updated-www")
		updatedUnstr, err := k8sutils.ToUnstructured(testSts)
		require.NoError(t, err)
		err = cli.Patch(context.Background(), updatedUnstr, client.Apply, client.FieldOwner("ollama-operator"))
		require.Error(t, err)
		require.True(t, IsImmutable(err))
	})
	t.Run("detects immutable fields when updating configMap with immutable=true", func(t *testing.T) {
		testCm := applycorev1.ConfigMap("test", "default").
			WithData(map[string]string{"foo": "bar"}).
			WithImmutable(true)
		unstr, err := k8sutils.ToUnstructured(testCm)
		require.NoError(t, err)

		err = cli.Patch(context.Background(), unstr, client.Apply, client.FieldOwner("ollama-operator"))
		require.NoError(t, err)

		testCm.Data = map[string]string{"foo": "bar-updated"}
		updatedUnstr, err := k8sutils.ToUnstructured(testCm)
		require.NoError(t, err)
		err = cli.Patch(context.Background(), updatedUnstr, client.Apply, client.FieldOwner("ollama-operator"))
		require.Error(t, err)
		require.True(t, IsImmutable(err))
	})
}
