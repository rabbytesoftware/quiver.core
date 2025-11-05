package reader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReader_ReadArrow(t *testing.T) {
	reader := NewReader()

	tmpDir := t.TempDir()
	arrowFile := filepath.Join(tmpDir, "arrow.yaml")

	arrowYAML := `manifest: "arrow@v1"
metadata:
  version: "1.0.0"
  license: "MIT"
  quiver_url: "https://example.com"
  name: "test-arrow"
  description: "Test Arrow"
  media:
    icon: "https://example.com/icon.png"
    banner: "https://example.com/banner.png"
  credits:
    - name: "Test User"
      email: "test@example.com"
      url: "https://example.com"
requirements:
  cpu_cores: 2
  ram_gb: 4
  disk_gb: 10
  network_mbps: 10
  system:
    - "linux/amd64"
netbridge:
  - name: "TEST_PORT"
    protocol: "tcp"
variables:
  - name: "TEST_VAR"
    default: "value"
    sensitive: false
methods:
  windows:
    amd64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
    arm64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
  linux:
    amd64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
    arm64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
  darwin:
    amd64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
    arm64:
      install: ["echo install"]
      execute: ["echo execute"]
      uninstall: ["echo uninstall"]
      update: ["echo update"]
      validate: ["echo validate"]
`

	if err := os.WriteFile(arrowFile, []byte(arrowYAML), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	arrow, err := reader.ReadArrow(arrowFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if arrow.Name != "test-arrow" {
		t.Errorf("got name %s, want test-arrow", arrow.Name)
	}
}

func TestReader_ReadManifestInfo(t *testing.T) {
	reader := NewReader()

	tmpDir := t.TempDir()
	manifestFile := filepath.Join(tmpDir, "manifest.yaml")

	manifestYAML := `manifest: "arrow@v1"
metadata:
  name: "test"
`

	if err := os.WriteFile(manifestFile, []byte(manifestYAML), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	info, err := reader.ReadManifestInfo(manifestFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.SchemaType != "arrow" {
		t.Errorf("got type %s, want arrow", info.SchemaType)
	}
	if info.Version != "v1" {
		t.Errorf("got version %s, want v1", info.Version)
	}
}
