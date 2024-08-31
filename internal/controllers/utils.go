package controller

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	DefaultOllamaPort              = 11434
	DefaultOllamaContainerImageTag = "ollama/ollama:0.3.3" // renovate: datasource=docker depName=ollama packageName=ollama/ollama
	DefaultOllamaContainerImage    = "ollama/ollama:" + DefaultOllamaContainerImageTag
)

func toUnstructured(obj any) (*unstructured.Unstructured, error) {
	unstr := &unstructured.Unstructured{}
	var err error
	unstr.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	return unstr, nil
}
