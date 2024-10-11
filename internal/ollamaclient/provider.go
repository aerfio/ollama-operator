package ollamaclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"

	ollamaapi "github.com/ollama/ollama/api"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"aerf.io/ollama-operator/internal/defaults"
)

func NewProvider(baseHTTPClient *http.Client, tracer trace.Tracer) *Provider {
	return &Provider{
		baseHTTPClient: baseHTTPClient,
		tracer:         tracer,
	}
}

type Provider struct {
	baseHTTPClient *http.Client
	tracer         trace.Tracer
}

func (p *Provider) ForModel(model metav1.Object) Interface {
	u := &url.URL{
		Scheme: "http",
		// use orbstack k8s locally to run that llm container
		Host: net.JoinHostPort(fmt.Sprintf("%s.%s.svc.cluster.local", model.GetName(), model.GetNamespace()), strconv.Itoa(defaults.OllamaPort)),
	}

	return NewTracingAwareClient(ollamaapi.NewClient(u, p.baseHTTPClient), p.tracer)
}
