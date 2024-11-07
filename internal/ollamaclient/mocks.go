package ollamaclient

import (
	"context"

	"github.com/ollama/ollama/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	TestOllamaClientProvider struct {
		Client *TestOllamaClient
	}
	TestOllamaClient struct {
		OnGenerate func(ctx context.Context, req *api.GenerateRequest, progressFunc api.GenerateResponseFunc) error
		OnList     func(ctx context.Context) (*api.ListResponse, error)
		OnPull     func(ctx context.Context, req *api.PullRequest, progressFunc api.PullProgressFunc) error
		OnShow     func(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error)
	}
)

var (
	_ ClientProvider = &TestOllamaClientProvider{}
	_ Interface      = &TestOllamaClient{}
)

func (t *TestOllamaClientProvider) ForModel(model metav1.Object) Interface {
	return t.Client
}

func (t *TestOllamaClient) Generate(ctx context.Context, req *api.GenerateRequest, progressFunc api.GenerateResponseFunc) error {
	return t.OnGenerate(ctx, req, progressFunc)
}

func (t *TestOllamaClient) List(ctx context.Context) (*api.ListResponse, error) {
	return t.OnList(ctx)
}

func (t *TestOllamaClient) Pull(ctx context.Context, req *api.PullRequest, progressFunc api.PullProgressFunc) error {
	return t.OnPull(ctx, req, progressFunc)
}

func (t *TestOllamaClient) Show(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error) {
	return t.OnShow(ctx, req)
}
