// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1alpha1

// ConfigMapKeySelectorApplyConfiguration represents a declarative configuration of the ConfigMapKeySelector type for use
// with apply.
type ConfigMapKeySelectorApplyConfiguration struct {
	ConfigMapReferenceApplyConfiguration `json:",inline"`
	Key                                  *string `json:"key,omitempty"`
}

// ConfigMapKeySelectorApplyConfiguration constructs a declarative configuration of the ConfigMapKeySelector type for use with
// apply.
func ConfigMapKeySelector() *ConfigMapKeySelectorApplyConfiguration {
	return &ConfigMapKeySelectorApplyConfiguration{}
}

// WithName sets the Name field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Name field is set to the value of the last call.
func (b *ConfigMapKeySelectorApplyConfiguration) WithName(value string) *ConfigMapKeySelectorApplyConfiguration {
	b.Name = &value
	return b
}

// WithNamespace sets the Namespace field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Namespace field is set to the value of the last call.
func (b *ConfigMapKeySelectorApplyConfiguration) WithNamespace(value string) *ConfigMapKeySelectorApplyConfiguration {
	b.Namespace = &value
	return b
}

// WithKey sets the Key field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Key field is set to the value of the last call.
func (b *ConfigMapKeySelectorApplyConfiguration) WithKey(value string) *ConfigMapKeySelectorApplyConfiguration {
	b.Key = &value
	return b
}