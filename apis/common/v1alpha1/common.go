// +kubebuilder:object:generate=true
package v1alpha1

import (
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// A ConfigMapReference is a reference to a configmap in an arbitrary namespace.
type ConfigMapReference struct {
	// Name of the configmap.
	Name string `json:"name"`

	// Namespace of the configmap.
	Namespace string `json:"namespace,omitempty"`
}

// A ConfigMapKeySelector is a reference to a configmap key in an arbitrary namespace.
type ConfigMapKeySelector struct {
	ConfigMapReference `json:",inline"`

	// The key to select.
	Key string `json:"key"`
}

type Patches struct {
	JSONPatch  `json:",inline"`
	MergePatch `json:",inline"`
}

type JSONPatch struct {
	// JSON Patch: https://datatracker.ietf.org/doc/html/rfc6902
	JSONPatch []JSONPatchOperation `json:"jsonPatch,omitempty"`
}

// https://datatracker.ietf.org/doc/html/rfc6902
// +kubebuilder:validation:XValidation:rule="((self.op in ['move', 'copy']) && has(self.from)) || (!(self.op in ['move', 'copy']) && !has(self.from))",message="The operation object MUST contain a 'from' member if the op is move or copy, in other cases it's forbidden"
// +kubebuilder:validation:XValidation:rule="((self.op in ['add', 'replace']) && has(self.value)) || (!(self.op in ['add', 'replace']) && !has(self.value))",message="The operation object MUST contain a 'value' member if the op is add or replace, in other cases it's forbidden"
type JSONPatchOperation struct {
	// +kubebuilder:validation:Enum=add;replace;remove;move;copy;test
	Op    string                `json:"op"`
	Path  string                `json:"path"`
	From  string                `json:"from,omitempty"`
	Value *runtime.RawExtension `json:"value,omitempty"`
}

type MergePatch struct {
	// JSON Merge Patch: https://datatracker.ietf.org/doc/html/rfc7386.
	// Note that as per RFC "it is not possible to patch part of a target that is not an object, such as to replace just some of the values in an array.". Use JSON MergePatch for that.
	// +kubebuilder:pruning:PreserveUnknownFields
	MergePatch *runtime.RawExtension `json:"mergePatch,omitempty"`
}

func (mp *MergePatch) PatchToUnstructured() (*unstructured.Unstructured, error) {
	if mp.MergePatch == nil {
		return nil, fmt.Errorf("json merge patch is nil")
	}
	var err error
	unstr := &unstructured.Unstructured{}
	unstr.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(mp.MergePatch)
	return unstr, errors.Wrap(err, "failed to convert patch to unstructured")
}
