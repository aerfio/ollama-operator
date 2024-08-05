package v1alpha1

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
