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
)

// ModelSpec defines the desired state of Model
type ModelSpec struct {
	// https://hub.docker.com/r/ollama/ollama/tags
	OllamaImage string `json:"ollamaImage,omitempty"`

	// Model like phi3, llama3.1 etc
	Model              string   `json:"model"`
	StatefulSetPatches *Patches `json:"statefulSetPatches,omitempty"`
	ServicePatches     *Patches `json:"servicePatches,omitempty"`
}

// ModelStatus defines the observed state of Model
type ModelStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	// ObservedGeneration is the latest metadata.generation
	// which resulted in either a ready state, or stalled due to error
	// it can not recover from without human intervention.
	// +optional
	ObservedGeneration int64               `json:"observedGeneration,omitempty"`
	OllamaImage        string              `json:"ollamaImage,omitempty"`
	OllamaModelDetails *OllamaModelDetails `json:"modelDetails,omitempty"`
}

type OllamaModelDetails struct {
	ParameterSize     string   `json:"parameterSize,omitempty"`
	QuantizationLevel string   `json:"quantizationLevel,omitempty"`
	ParentModel       string   `json:"parentModel,omitempty"`
	Format            string   `json:"format,omitempty"`
	Family            string   `json:"family,omitempty"`
	Families          []string `json:"families,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MODEL",type="string",JSONPath=".spec.model"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="PARAMETER_SIZE",type="string",JSONPath=".status.modelDetails.parameterSize"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Namespaced,categories={ollama}

// Model is the Schema for the models API
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec,omitempty"`
	Status ModelStatus `json:"status,omitempty"`
}

func (in *Model) SetConditionsWithObservedGeneration(c ...xpv1.Condition) {
	for i := range c {
		c[i].ObservedGeneration = in.Generation
	}

	in.Status.SetConditions(c...)
}

func (in *Model) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return in.Status.GetCondition(ct)
}

// +kubebuilder:object:root=true

// ModelList contains a list of Model
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}
