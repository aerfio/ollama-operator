/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PromptSpec defines the desired state of Prompt
type PromptSpec struct {
	ModelRef ModelRef `json:"modelRef"`

	Prompt string `json:"prompt"`

	// Context is the context returned from previous prompt. Copy it from .status.context of previously run prompt. Optional field
	Context string `json:"context,omitempty"`
	// Suffix is the text that comes after the inserted text.
	Suffix string `json:"suffix,omitempty"`

	// System overrides the model's default system message/prompt.
	System string `json:"system,omitempty"`

	// Template overrides the model's default prompt template.
	Template string `json:"template,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Options runtime.RawExtension `json:"options,omitempty"`

	Images []ImageSource `json:"images,omitempty"`
}

// TODO cel expression that only 1 field should be set
type ImageSource struct {
	Inline          *ImageData              `json:"inline,omitempty"`
	SecretKeyRef    *xpv1.SecretKeySelector `json:"secretKeyRef,omitempty"`
	ConfigMapKeyRef *ConfigMapKeySelector   `json:"configMapKeyRef,omitempty"`
}

// +kubebuilder:validation:Enum=gzip;zstd;none
type ImageFormat string

const (
	ImageFormatNone ImageFormat = "none"
	ImageFormatGzip ImageFormat = "gzip"
	ImageFormatZstd ImageFormat = "zstd"
)

var ImageFormatAll = []ImageFormat{ImageFormatGzip, ImageFormatZstd, ImageFormatNone}

type ImageData struct {
	Format ImageFormat `json:"format,omitempty"`
	Data   string      `json:"data"`
}

type ModelRef struct {
	Name string `json:"name"`
	// defaults to prompt namespace
	Namespace string `json:"namespace,omitempty"`
}

// PromptStatus defines the observed state of Model
type PromptStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	// ObservedGeneration is the latest metadata.generation
	// which resulted in either a ready state, or stalled due to error
	// it can not recover from without human intervention.
	// +optional
	ObservedGeneration    int64                  `json:"observedGeneration,omitempty"`
	Response              string                 `json:"response,omitempty"`
	Context               string                 `json:"context,omitempty"`
	PromptResponseMeta    *PromptResponseMeta    `json:"meta,omitempty"`
	PromptResponseMetrics *PromptResponseMetrics `json:"metrics,omitempty"`
}

type PromptResponseMeta struct {
	CreatedAt metav1.Time `json:"createdAt,omitempty"`
}

type PromptResponseMetrics struct {
	TotalDuration      metav1.Duration `json:"totalDuration,omitempty"`
	LoadDuration       metav1.Duration `json:"loadDuration,omitempty"`
	PromptEvalCount    int64           `json:"promptEvalCount,omitempty"`
	PromptEvalDuration metav1.Duration `json:"promptEvalDuration,omitempty"`
	PromptEvalRate     string          `json:"promptEvalRate,omitempty"`
	EvalCount          int64           `json:"evalCount,omitempty"`
	EvalDuration       metav1.Duration `json:"evalDuration,omitempty"`
	EvalRate           string          `json:"evalRate,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MODEL_REF_NAME",type="string",JSONPath=".spec.modelRef.name"
// +kubebuilder:printcolumn:name="MODEL_REF_NAMESPACE",type="string",JSONPath=".spec.modelRef.name"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={ollama}

// Prompt is the Schema for the models API
type Prompt struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PromptSpec   `json:"spec,omitempty"`
	Status PromptStatus `json:"status,omitempty"`
}

func (in *Prompt) SetConditionsWithObservedGeneration(c ...xpv1.Condition) {
	for i := range c {
		c[i].ObservedGeneration = in.Generation
	}

	in.Status.SetConditions(c...)
}

func (in *Prompt) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return in.Status.GetCondition(ct)
}

// +kubebuilder:object:root=true

// PromptList contains a list of Prompt
type PromptList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Prompt `json:"items"`
}
