// Package v1alpha1 contains API Schema definitions for the system v1alpha1 API group
// FIXME: https://github.com/kubernetes/code-generator/issues/150
// This was the only setup I could make work. Somehow the file location matters.
// @aerfio solution copied from https://github.com/statnett/image-scanner-operator, thank you!
// +kubebuilder:object:generate=true
// +groupName=ollama.aerf.io
// +kubebuilder:ac:generate=true
// +kubebuilder:ac:output:package="applyconfiguration"
package v1alpha1
