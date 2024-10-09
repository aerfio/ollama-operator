package applyconfig

import (
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ControllerReferenceFrom(obj client.Object) *applymetav1.OwnerReferenceApplyConfiguration {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return applymetav1.OwnerReference().
		WithName(obj.GetName()).
		WithKind(gvk.Kind).
		WithAPIVersion(gvk.GroupVersion().String()).
		WithUID(obj.GetUID()).
		WithBlockOwnerDeletion(true).
		WithController(true)
}
