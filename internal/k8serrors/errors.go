package k8serrors

import (
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func IsImmutable(err error) bool {
	// logic is based on github.com/fluxcd/pkg and on kubectl codebase
	if err == nil {
		return false
	}
	if apierrors.IsInvalid(err) {
		return true
	}
	if strings.Contains(err.Error(), "is immutable") {
		return true
	}
	return false
}
