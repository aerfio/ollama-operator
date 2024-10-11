package applyconfig

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	applymetav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/utils/ptr"
)

func TestControllerReferenceFrom(t *testing.T) {
	object := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "name",
			UID:       types.UID("uid"),
		},
	}
	expected := &applymetav1.OwnerReferenceApplyConfiguration{
		APIVersion:         ptr.To("v1"),
		Kind:               ptr.To("ConfigMap"),
		Name:               ptr.To("name"),
		UID:                ptr.To(types.UID("uid")),
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
	if diff := cmp.Diff(expected, ControllerReferenceFrom(object)); diff != "" {
		t.Fatalf("expected no diff; -got +want:\n%s", diff)
	}
}
