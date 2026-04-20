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
func nsFor(
	fixture string,
	tag string,
) string {
	return "quiver.test/" + fixture + "@" + tag
}

// arrowsTestdataDir returns the absolute path to the testdata/arrows/ directory.
func arrowsTestdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "arrows")
}

// readFixture reads a file from testdata/arrows/ relative to the test package.
func readFixture(
	t *testing.T,
	relPath string,
) []byte {
	t.Helper()
	abs := filepath.Join(arrowsTestdataDir(), relPath)
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("readFixture(%q): %v", relPath, err)
	}
	return data
}
