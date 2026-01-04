package apilibs

import (
	"os"
	"testing"
)

func TestNewApiLib(t *testing.T) {
	lib := NewApiLib()
	if lib == nil {
		t.Fatal("NewApiLib returned nil")
	}
}

func TestIsDirectory_ValidDirectory(t *testing.T) {
	lib := NewApiLib()

	tmpDir, err := os.MkdirTemp("", "test_dir_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !lib.IsDirectory(tmpDir) {
		t.Fatalf("IsDirectory returned false for valid directory: %s", tmpDir)
	}
}

func TestIsDirectory_InvalidPath(t *testing.T) {
	lib := NewApiLib()

	if lib.IsDirectory("/nonexistent/path/that/does/not/exist") {
		t.Fatal("IsDirectory returned true for non-existent path")
	}
}

func TestIsDirectory_File(t *testing.T) {
	lib := NewApiLib()

	tmpFile, err := os.CreateTemp("", "test_file_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if lib.IsDirectory(tmpFile.Name()) {
		t.Fatalf("IsDirectory returned true for file: %s", tmpFile.Name())
	}
}

func TestIsUrl_ValidHttps(t *testing.T) {
	lib := NewApiLib()

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
		if !lib.IsUrl(url) {
			t.Fatalf("IsUrl returned false for valid URL: %s", url)
		}
	}
}

func TestIsUrl_InvalidUrls(t *testing.T) {
	lib := NewApiLib()

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
		if lib.IsUrl(url) {
			t.Fatalf("IsUrl returned true for invalid URL: %s", url)
		}
	}
}

func TestIsNamespace_Valid(t *testing.T) {
	lib := NewApiLib()

	tests := []string{
		"QUID:AUID",
		"my-quiver:my-arrow",
		"q1:a1",
	}

	for _, ns := range tests {
		if !lib.IsNamespace(ns, "") {
			t.Fatalf("IsNamespace returned false for valid namespace: %s", ns)
		}
	}
}

func TestIsNamespace_Invalid(t *testing.T) {
	lib := NewApiLib()

	tests := []string{
		"",
		"QUID",
		":AUID",
		"QUID:",
		"QUID:AUID:extra",
	}

	for _, ns := range tests {
		if lib.IsNamespace(ns, "") {
			t.Fatalf("IsNamespace returned true for invalid namespace: %s", ns)
		}
	}
}
