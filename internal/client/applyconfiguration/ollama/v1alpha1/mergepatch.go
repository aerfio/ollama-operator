// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// MergePatchApplyConfiguration represents a declarative configuration of the MergePatch type for use
// with apply.
type MergePatchApplyConfiguration struct {
	MergePatch *runtime.RawExtension `json:"mergePatch,omitempty"`
}

// MergePatchApplyConfiguration constructs a declarative configuration of the MergePatch type for use with
// apply.
func MergePatch() *MergePatchApplyConfiguration {
	return &MergePatchApplyConfiguration{}
}

// WithMergePatch sets the MergePatch field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the MergePatch field is set to the value of the last call.
func (b *MergePatchApplyConfiguration) WithMergePatch(value runtime.RawExtension) *MergePatchApplyConfiguration {
	b.MergePatch = &value
	return b
}