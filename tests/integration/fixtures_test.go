//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// nsFor constructs a test namespace for the given fixture and tag.
// fixture: relative path under testdata/arrows/ e.g. "quiver-test/tool-a" or "dep-diamond/root"
// tag: git tag e.g. "v1"
// Returns: "quiver.test/quiver-test/tool-a@v1"
func nsFor(fixture, tag string) string {
	return "quiver.test/" + fixture + "@" + tag
}

// readFixture reads a file from testdata/arrows/ relative to the test package.
func readFixture(t *testing.T, relPath string) []byte {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	abs := filepath.Join(filepath.Dir(file), "testdata", "arrows", relPath)
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("readFixture(%q): %v", relPath, err)
	}
	return data
}
