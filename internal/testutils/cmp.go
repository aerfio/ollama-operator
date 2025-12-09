package testutils

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func IgnoreXPv1ConditionFields(additionalIgnoredFields ...string) cmp.Option {
	ignoredFields := []string{"LastTransitionTime", "ObservedGeneration"}
	ignoredFields = append(ignoredFields, additionalIgnoredFields...)
	return cmpopts.IgnoreFields(xpv1.Condition{}, ignoredFields...)
}
