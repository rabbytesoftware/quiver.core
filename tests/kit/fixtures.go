//go:build integration

package kit

import (
	"os"
	"path/filepath"
	"testing"
)

// ReadFixture reads a file from testdata/arrows/ relative to the calling suite's
// working directory. All suite packages live one level below tests/integration/,
// so ../testdata/arrows/<relPath> always resolves correctly.
func ReadFixture(t *testing.T, relPath string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "arrows", relPath)) // #nosec G304 -- path is under testdata/, controlled by test fixtures only
	if err != nil {
		t.Fatalf("ReadFixture(%q): %v", relPath, err)
	}
	return data
}
