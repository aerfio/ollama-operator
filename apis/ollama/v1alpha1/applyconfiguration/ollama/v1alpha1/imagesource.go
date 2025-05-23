// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// ImageSourceApplyConfiguration represents a declarative configuration of the ImageSource type for use
// with apply.
type ImageSourceApplyConfiguration struct {
	Inline          *ImageDataApplyConfiguration            `json:"inline,omitempty"`
	SecretKeyRef    *v1.SecretKeySelector                   `json:"secretKeyRef,omitempty"`
	ConfigMapKeyRef *ConfigMapKeySelectorApplyConfiguration `json:"configMapKeyRef,omitempty"`
}

// ImageSourceApplyConfiguration constructs a declarative configuration of the ImageSource type for use with
// apply.
func ImageSource() *ImageSourceApplyConfiguration {
	return &ImageSourceApplyConfiguration{}
}

// WithInline sets the Inline field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Inline field is set to the value of the last call.
func (b *ImageSourceApplyConfiguration) WithInline(value *ImageDataApplyConfiguration) *ImageSourceApplyConfiguration {
	b.Inline = value
	return b
}

// WithSecretKeyRef sets the SecretKeyRef field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the SecretKeyRef field is set to the value of the last call.
func (b *ImageSourceApplyConfiguration) WithSecretKeyRef(value v1.SecretKeySelector) *ImageSourceApplyConfiguration {
	b.SecretKeyRef = &value
	return b
}

// WithConfigMapKeyRef sets the ConfigMapKeyRef field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ConfigMapKeyRef field is set to the value of the last call.
func (b *ImageSourceApplyConfiguration) WithConfigMapKeyRef(value *ConfigMapKeySelectorApplyConfiguration) *ImageSourceApplyConfiguration {
	b.ConfigMapKeyRef = value
	return b
}
