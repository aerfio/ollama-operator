package generic

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

type ExternalClient[T resource.Managed] interface {
	// Observe the external resource the supplied Managed resource
	// represents, if any. Observe implementations must not modify the
	// external resource, but may update the supplied Managed resource to
	// reflect the state of the external resource. Status modifications are
	// automatically persisted unless ResourceLateInitialized is true - see
	// ResourceLateInitialized for more detail.
	Observe(ctx context.Context, mg T) (managed.ExternalObservation, error)

	// Create an external resource per the specifications of the supplied
	// Managed resource. Called when Observe reports that the associated
	// external resource does not exist. Create implementations may update
	// managed resource annotations, and those updates will be persisted.
	// All other updates will be discarded.
	Create(ctx context.Context, mg T) (managed.ExternalCreation, error)

	// Update the external resource represented by the supplied Managed
	// resource, if necessary. Called unless Observe reports that the
	// associated external resource is up to date.
	Update(ctx context.Context, mg T) (managed.ExternalUpdate, error)

	// Delete the external resource upon deletion of its associated Managed
	// resource. Called when the managed resource has been deleted.
	Delete(ctx context.Context, mg T) (managed.ExternalDelete, error)
}

func NewExternalForType[T resource.Managed](specificExternal ExternalClient[T], errNotConfiguredType error) *ExternalAdapter[T] {
	return &ExternalAdapter[T]{
		genericExternal:      specificExternal,
		errNotConfiguredType: errNotConfiguredType,
	}
}

type ExternalAdapter[T resource.Managed] struct {
	genericExternal      ExternalClient[T]
	errNotConfiguredType error
}

func (g *ExternalAdapter[T]) Disconnect(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (g *ExternalAdapter[T]) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(T)
	if !ok {
		return managed.ExternalObservation{}, g.errNotConfiguredType
	}

	return g.genericExternal.Observe(ctx, cr)
}

func (g *ExternalAdapter[T]) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(T)
	if !ok {
		return managed.ExternalCreation{}, g.errNotConfiguredType
	}

	return g.genericExternal.Create(ctx, cr)
}

func (g *ExternalAdapter[T]) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(T)
	if !ok {
		return managed.ExternalUpdate{}, g.errNotConfiguredType
	}

	return g.genericExternal.Update(ctx, cr)
}

func (g *ExternalAdapter[T]) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(T)
	if !ok {
		return managed.ExternalDelete{}, g.errNotConfiguredType
	}

	return g.genericExternal.Delete(ctx, cr)
}
