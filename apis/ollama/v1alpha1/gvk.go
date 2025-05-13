package v1alpha1

const (
	ModelKind  = "Model"
	PromptKind = "Prompt"
)

var (
	ModelGroupVersionKind  = SchemeGroupVersion.WithKind(ModelKind)
	PromptGroupVersionKind = SchemeGroupVersion.WithKind(PromptKind)
)
