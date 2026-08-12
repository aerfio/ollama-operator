package testutils

import (
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func IgnoreXPv1ConditionFields(additionalIgnoredFields ...string) cmp.Option {
	ignoredFields := make([]string, 0, 2+len(additionalIgnoredFields))
	ignoredFields = append(ignoredFields, "LastTransitionTime", "ObservedGeneration")
	ignoredFields = append(ignoredFields, additionalIgnoredFields...)
	return cmpopts.IgnoreFields(xpv2.Condition{}, ignoredFields...)
}
