// Code generated by applyconfiguration-gen. DO NOT EDIT.

package applyconfiguration

import (
	v1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	internal "aerf.io/ollama-operator/internal/client/applyconfiguration/internal"
	ollamav1alpha1 "aerf.io/ollama-operator/internal/client/applyconfiguration/ollama/v1alpha1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	testing "k8s.io/client-go/testing"
)

// ForKind returns an apply configuration type for the given GroupVersionKind, or nil if no
// apply configuration type exists for the given GroupVersionKind.
func ForKind(kind schema.GroupVersionKind) interface{} {
	switch kind {
	// Group=ollama.aerf.io, Version=v1alpha1
	case v1alpha1.SchemeGroupVersion.WithKind("ConfigMapKeySelector"):
		return &ollamav1alpha1.ConfigMapKeySelectorApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ConfigMapReference"):
		return &ollamav1alpha1.ConfigMapReferenceApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ImageData"):
		return &ollamav1alpha1.ImageDataApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ImageSource"):
		return &ollamav1alpha1.ImageSourceApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("JSONPatch"):
		return &ollamav1alpha1.JSONPatchApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("JSONPatchOperation"):
		return &ollamav1alpha1.JSONPatchOperationApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("MergePatch"):
		return &ollamav1alpha1.MergePatchApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("Model"):
		return &ollamav1alpha1.ModelApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ModelRef"):
		return &ollamav1alpha1.ModelRefApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ModelSpec"):
		return &ollamav1alpha1.ModelSpecApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("ModelStatus"):
		return &ollamav1alpha1.ModelStatusApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("OllamaModelDetails"):
		return &ollamav1alpha1.OllamaModelDetailsApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("Patches"):
		return &ollamav1alpha1.PatchesApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("Prompt"):
		return &ollamav1alpha1.PromptApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("PromptResponseMeta"):
		return &ollamav1alpha1.PromptResponseMetaApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("PromptResponseMetrics"):
		return &ollamav1alpha1.PromptResponseMetricsApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("PromptSpec"):
		return &ollamav1alpha1.PromptSpecApplyConfiguration{}
	case v1alpha1.SchemeGroupVersion.WithKind("PromptStatus"):
		return &ollamav1alpha1.PromptStatusApplyConfiguration{}

	}
	return nil
}

func NewTypeConverter(scheme *runtime.Scheme) *testing.TypeConverter {
	return &testing.TypeConverter{Scheme: scheme, TypeResolver: internal.Parser()}
}
