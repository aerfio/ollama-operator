package ollamaclient

import (
	"context"

	ollamaapi "github.com/ollama/ollama/api"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"aerf.io/k8sutils/k8stracing"
)

type Interface interface {
	Show(ctx context.Context, req *ollamaapi.ShowRequest) (*ollamaapi.ShowResponse, error)
	List(ctx context.Context) (*ollamaapi.ListResponse, error)
	Pull(ctx context.Context, req *ollamaapi.PullRequest, progressFunc ollamaapi.PullProgressFunc) error
	Generate(ctx context.Context, req *ollamaapi.GenerateRequest, progressFunc ollamaapi.GenerateResponseFunc) error
}

func NewTracingAwareClient(wrapped Interface, tracer trace.Tracer) Interface {
	return &tracingAwareClient{
		wrapped: wrapped,
		tracer:  tracer,
	}
}

type tracingAwareClient struct {
	wrapped Interface
	tracer  trace.Tracer
}

func (t *tracingAwareClient) Generate(ctx context.Context, req *ollamaapi.GenerateRequest, progressFunc ollamaapi.GenerateResponseFunc) error {
	ctx, span := t.tracer.Start(ctx, "generate")
	defer span.End()

	err := t.wrapped.Generate(ctx, req, progressFunc)
	if err != nil {
		k8stracing.SetSpanErr(span, err)
		return err
	}
	span.SetStatus(codes.Ok, "success")
	return nil
}

func (t *tracingAwareClient) List(ctx context.Context) (*ollamaapi.ListResponse, error) {
	ctx, span := t.tracer.Start(ctx, "list")
	defer span.End()

	resp, err := t.wrapped.List(ctx)
	if err != nil {
		k8stracing.SetSpanErr(span, err)
		return resp, err
	}
	span.SetStatus(codes.Ok, "success")
	return resp, nil
}

func (t *tracingAwareClient) Pull(ctx context.Context, req *ollamaapi.PullRequest, progressFunc ollamaapi.PullProgressFunc) error {
	ctx, span := t.tracer.Start(ctx, "pull")
	defer span.End()

	err := t.wrapped.Pull(ctx, req, progressFunc)
	if err != nil {
		k8stracing.SetSpanErr(span, err)
		return err
	}
	span.SetStatus(codes.Ok, "success")
	return nil
}

func (t *tracingAwareClient) Show(ctx context.Context, req *ollamaapi.ShowRequest) (*ollamaapi.ShowResponse, error) {
	ctx, span := t.tracer.Start(ctx, "show")
	defer span.End()

	resp, err := t.wrapped.Show(ctx, req)
	if err != nil {
		k8stracing.SetSpanErr(span, err)
		return resp, err
	}
	span.SetStatus(codes.Ok, "success")
	return resp, nil
}
