package patches_test

import (
	"strings"
	"testing"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/yaml"

	cmnv1alpha1 "aerf.io/ollama-operator/apis/common/v1alpha1"
	"aerf.io/ollama-operator/internal/patches"
)

func TestApplyMerge(t *testing.T) {
	tests := map[string]struct {
		obj     any
		patch   string
		want    any
		errPart string
	}{
		"PatchesExistingFieldsAndNew": {
			obj: &applycorev1.PodApplyConfiguration{
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Labels: map[string]string{
						"key": "value",
					},
				},
			},
			patch: `
mergePatch:
  metadata:
    labels:
      key: value2
    annotations:
      annotationKey: annoValue`,
			want: &applycorev1.PodApplyConfiguration{
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Labels: map[string]string{
						"key": "value2",
					},
					Annotations: map[string]string{
						"annotationKey": "annoValue",
					},
				},
			},
		},
		"FailsOnPatchToNonExistentFieldInSource": {
			obj: &applycorev1.PodApplyConfiguration{
				ObjectMetaApplyConfiguration: &applymetav1.ObjectMetaApplyConfiguration{
					Labels: map[string]string{
						"key": "value",
					},
				},
			},
			patch: `
mergePatch:
  metadata:
    something: true`,
			errPart: `unknown field "metadata.something"`,
		},
		"AlwaysNoErrNoChangesOnNilPatch": {
			obj:     applycorev1.Namespace("ns"),
			patch:   "",
			want:    applycorev1.Namespace("ns"),
			errPart: "",
		},
		"WorkOnUnstructuredTypes": {
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"labels": map[string]any{
							"key": "value",
						},
					},
				},
			},
			patch: `
mergePatch:
  metadata:
    labels:
      key2: value2`,
			want: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"labels": map[string]any{
							"key":  "value",
							"key2": "value2",
						},
					},
				},
			},
			errPart: "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ptch := &cmnv1alpha1.MergePatch{}
			if tt.patch != "" {
				require.NoError(t, yaml.Unmarshal([]byte(strings.NewReplacer("\t", "  ").Replace(tt.patch)), ptch))
			} else {
				ptch = nil
			}

			merged, err := patches.ApplyMerge(tt.obj, ptch)
			if tt.errPart != "" {
				require.ErrorContains(t, err, tt.errPart)
				return
			}
			require.NoError(t, err)

			if diff := cmp.Diff(merged, tt.want); diff != "" {
				t.Fatalf("Result differs from expected, -got, +want:\n%s", diff)
			}
		})
	}
}

func TestApplyJSONPatch(t *testing.T) {
	tests := map[string]struct {
		obj      any
		patch    string
		want     any
		errParts []string
	}{
		"NoErrOnSimpleReplacePatch": {
			obj: applycorev1.Namespace("ns").
				WithLabels(map[string]string{
					"key": "value",
				}),
			patch: `jsonPatch:
- op: replace
  path: /metadata/labels/key
  value: value2`,
			want: applycorev1.Namespace("ns").
				WithLabels(map[string]string{
					"key": "value2",
				}),
		},
		"NoErrOnNilPatch": {
			obj: applycorev1.Namespace("ns").
				WithLabels(map[string]string{
					"key": "value",
				}),
			patch: "",
			want: applycorev1.Namespace("ns").
				WithLabels(map[string]string{
					"key": "value",
				}),
		},
		"WorksWithUnstructured": {
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"labels": map[string]any{
							"key": "value",
						},
					},
				},
			},
			patch: `jsonPatch:
- op: add
  path: /spec/serviceAccountName
  value: sa-name
- op: replace
  path: /metadata/labels/key
  value: value2`,
			want: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"labels": map[string]any{
							"key": "value2",
						},
					},
					"spec": map[string]any{
						"serviceAccountName": "sa-name",
					},
				},
			},
		},
		"FailsToAddFieldsWhichDoNotExistOnTypeInConcreteType": {
			obj: applycorev1.Namespace("ns").
				WithLabels(map[string]string{
					"key": "value",
				}),
			patch: `jsonPatch:
- op: replace
  path: /metadata/something
  value: test-val`,
			errParts: []string{jsonpatch.ErrMissing.Error(), "replace operation does not apply"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ptch := &cmnv1alpha1.JSONPatch{}
			if tt.patch != "" {
				require.NoError(t, yaml.Unmarshal([]byte(strings.NewReplacer("\t", "  ").Replace(tt.patch)), ptch))
			} else {
				ptch = nil
			}

			jsonPatched, err := patches.ApplyJSONPatch(tt.obj, ptch)
			if len(tt.errParts) > 0 {
				for _, errPart := range tt.errParts {
					require.ErrorContains(t, err, errPart)
				}
				return
			}
			require.NoError(t, err)

			if diff := cmp.Diff(jsonPatched, tt.want); diff != "" {
				t.Fatalf("Result differs from expected, -got, +want:\n%s", diff)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := map[string]struct {
		resource string
		want     string
		patches  *cmnv1alpha1.Patches
		wantErr  bool
	}{
		"SimplePodPatchWithUnstructured": {
			resource: `
apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: null
  labels:
    run: nginx
  name: nginx
spec:
  containers:
  - image: nginx
    name: nginx
    resources: {}
  dnsPolicy: ClusterFirst
  restartPolicy: Always
status: {}
`,
			patches: &cmnv1alpha1.Patches{
				JSONPatch: cmnv1alpha1.JSONPatch{
					JSONPatch: []cmnv1alpha1.JSONPatchOperation{
						{
							Op:    "add",
							Path:  "/metadata/labels/app",
							Value: &runtime.RawExtension{Raw: []byte(`"test-value"`)},
						},
					},
				},
				MergePatch: cmnv1alpha1.MergePatch{
					MergePatch: &runtime.RawExtension{Raw: []byte(`{"spec":{"dnsPolicy": "ClusterFirstWithHostNet"}}`)},
				},
			},
			want: `apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: null
  labels:
    run: nginx
    app: test-value
  name: nginx
spec:
  containers:
  - image: nginx
    name: nginx
    resources: {}
  dnsPolicy: ClusterFirstWithHostNet
  restartPolicy: Always
status: {}`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			res := &unstructured.Unstructured{}
			require.NoError(t, yaml.UnmarshalStrict([]byte(tt.resource), res))

			want := &unstructured.Unstructured{}
			require.NoError(t, yaml.UnmarshalStrict([]byte(tt.want), want))

			patched, err := patches.Apply(res, tt.patches)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Apply() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(patched, want); diff != "" {
				t.Fatalf("Expected result differs from expected, -got, +want:\n%s", diff)
			}
		})
	}
}
