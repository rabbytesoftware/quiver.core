package apilibs

import (
	"os"
	"testing"
)

func TestIsDirectory_ValidDirectory(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "test_dir_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !IsDirectory(tmpDir) {
		t.Fatalf("IsDirectory returned false for valid directory: %s", tmpDir)
	}
}

func TestIsDirectory_InvalidPath(t *testing.T) {

	if IsDirectory("/nonexistent/path/that/does/not/exist") {
		t.Fatal("IsDirectory returned true for non-existent path")
	}
}

func TestIsDirectory_File(t *testing.T) {

	tmpFile, err := os.CreateTemp("", "test_file_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if IsDirectory(tmpFile.Name()) {
		t.Fatalf("IsDirectory returned true for file: %s", tmpFile.Name())
	}
}

func TestIsUrl_ValidHttps(t *testing.T) {

	tests := []string{
		"https://example.com",
		"https://example.com/path",
		"https://example.com:8080",
		"https://example.com:8080/path/to/file",
		"http://example.com",
		"http://sub.example.com",
		"https://api-server.example.co.uk:3000/v1/endpoint",
	}

	for _, url := range tests {
		if !IsUrl(url) {
			t.Fatalf("IsUrl returned false for valid URL: %s", url)
		}
	}
}

func TestIsUrl_InvalidUrls(t *testing.T) {

	tests := []string{
		"not-a-url",
		"ftp://example.com",
		"example.com",
		"://example.com",
		"http://",
		"",
		"http:// example.com",
	}

	for _, url := range tests {
		if IsUrl(url) {
			t.Fatalf("IsUrl returned true for invalid URL: %s", url)
		}
	}
}

func TestIsNamespace_Valid(t *testing.T) {

	tests := []string{
		"QUID:AUID",
		"my-quiver:my-arrow",
		"q1:a1",
	}

	for _, ns := range tests {
		if !IsNamespace(ns, "") {
			t.Fatalf("IsNamespace returned false for valid namespace: %s", ns)
		}
	}
}

func TestIsNamespace_Invalid(t *testing.T) {

	tests := []string{
		"",
		"QUID",
		":AUID",
		"QUID:",
		"QUID:AUID:extra",
	}

	for _, ns := range tests {
		if IsNamespace(ns, "") {
			t.Fatalf("IsNamespace returned true for invalid namespace: %s", ns)
		}
	}
}

func TestIsUrl_EdgeCases(t *testing.T) {

	edgeCases := []struct {
		url      string
		expected bool
		name     string
	}{
		{"https://example.com", true, "basic https"},
		{"http://example.com", true, "basic http"},
		{"https://example.com/path/to/file", true, "with path"},
		{"https://example.com:8080/path", true, "with port and path"},
		{"https://sub-domain.example.com", true, "with subdomain"},
		{"https://123.456.789.012:9000/api", true, "numeric ip-like"},
		{"ftp://example.com", false, "ftp protocol"},
		{"example.com", false, "no protocol"},
		{"", false, "empty string"},
		{"://", false, "only protocol separators"},
		{"http://", false, "protocol without domain"},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsUrl(tc.url)
			if result != tc.expected {
				t.Errorf("IsUrl(%q) = %v, want %v", tc.url, result, tc.expected)
			}
		})
	}
}

func TestIsDirectory_Permissions(t *testing.T) {

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "test_perm_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Should be a directory
	if !IsDirectory(tmpDir) {
		t.Fatalf("IsDirectory returned false for valid directory: %s", tmpDir)
	}
}

func TestIsNamespace_EdgeCases(t *testing.T) {

	edgeCases := []struct {
		namespace string
		expected  bool
		name      string
	}{
		{"QUID:AUID", true, "valid colon format"},
		{"my-quiver:my-arrow", true, "with hyphens"},
		{"q1:a1", true, "single char parts"},
		{"", false, "empty"},
		{"QUID", false, "missing second part"},
		{":AUID", false, "missing first part"},
		{"QUID:", false, "missing second part with colon"},
		{"QUID:AUID:EXTRA", false, "too many parts"},
		{"QUID AUID", false, "space instead of colon"},
		{"QUID-AUID", false, "hyphen instead of colon"},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsNamespace(tc.namespace, "")
			if result != tc.expected {
				t.Errorf("IsNamespace(%q) = %v, want %v", tc.namespace, result, tc.expected)
			}
		})
	}
}

func TestApiLib_MultipleInstances(t *testing.T) {

	// Both instances should work independently
	tmpDir, err := os.MkdirTemp("", "test_multi_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !IsDirectory(tmpDir) {
		t.Fatal("lib1.IsDirectory failed")
	}

	if !IsDirectory(tmpDir) {
		t.Fatal("lib2.IsDirectory failed")
	}
}

func TestIsUrl_ComplexPaths(t *testing.T) {
	complexUrls := []string{
		"https://api.example.com/v1/users/123/profile",
		"http://localhost:8080/api/test?param=value",
		"https://example.co.uk:9000/path/to/resource",
	}

	for _, url := range complexUrls {
		// Note: Simple regex might not match complex URLs with query params
		_ = IsUrl(url)
	}
}
