package testutils

import (
	"path/filepath"
	"runtime"
	"testing"
)

func GetCRDsDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("failed to get caller filename")
	}

	return filepath.Join(filepath.Dir(filename), "..", "..", "crds")
}
