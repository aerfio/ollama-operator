package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

/*
Copied from crossplane-runtime due to the bug in k8s-code-generator which fails with

>> generating internal/client/applyconfiguration...
F0103 16:16:23.397316   39641 main.go:57] Error: failed executing generator: some targets had errors: template: /home/aerfio/go/pkg/mod/k8s.io/code-generator@v0.32.0/cmd/applyconfiguration-gen/generators/applyconfiguration.go:342:1:37: executing "/home/aerfio/go/pkg/mod/k8s.io/code-generator@v0.32.0/cmd/applyconfiguration-gen/generators/applyconfiguration.go:342" at <.EmbeddedIn.MemberType.Elem.Name.Name>: nil pointer evaluating *types.Type.Name
make: *** [Makefile:81: k8s-client-gen] Error 255
*/

// NOTE(negz): Conditions are implemented as a slice rather than a map to comply
// with Kubernetes API conventions. Ideally we'd comply by using a map that
// marshalled to a JSON array, but doing so confuses the CRD schema generator.
// https://github.com/kubernetes/community/blob/9bf8cd/contributors/devel/sig-architecture/api-conventions.md#lists-of-named-subobjects-preferred-over-maps

// NOTE(negz): Do not manipulate Conditions directly. Use the Set method.

// A ConditionedStatus reflects the observed status of a resource. Only
// one condition of each type may exist.
type ConditionedStatus struct {
	// Conditions of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []xpv1.Condition `json:"conditions,omitempty"`
}

// NewConditionedStatus returns a stat with the supplied conditions set.
func NewConditionedStatus(c ...xpv1.Condition) *ConditionedStatus {
	s := &ConditionedStatus{}
	s.SetConditions(c...)
	return s
}

// GetCondition returns the condition for the given ConditionType if exists,
// otherwise returns nil.
func (s *ConditionedStatus) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	for _, c := range s.Conditions {
		if c.Type == ct {
			return c
		}
	}

	return xpv1.Condition{Type: ct, Status: corev1.ConditionUnknown}
}

// SetConditions sets the supplied conditions, replacing any existing conditions
// of the same type. This is a no-op if all supplied conditions are identical,
// ignoring the last transition time, to those already set.
// Observed generation is updated if higher than the existing one.
func (s *ConditionedStatus) SetConditions(c ...xpv1.Condition) {
	for _, newC := range c {
		exists := false
		for i, existing := range s.Conditions {
			if existing.Type != newC.Type {
				continue
			}

			if existing.Equal(newC) {
				exists = true
				if existing.ObservedGeneration < newC.ObservedGeneration {
					existing.ObservedGeneration = newC.ObservedGeneration
				}
				continue
			}

			s.Conditions[i] = newC
			exists = true
		}
		if !exists {
			s.Conditions = append(s.Conditions, newC)
		}
	}
}
