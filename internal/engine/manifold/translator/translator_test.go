package translator

import (
	"testing"
)

var validArrowV0 = []byte(`
schema: "arrow@v0"
name: test-arrow
description: A test arrow
version: 1.0.0
license: MIT
maintainers:
  - alice
tags:
  - test
requirements:
  cpu_cores: 2
  ram_gb: 4
  disk_gb: 10
  os:
    - linux/amd64
    - linux/arm64
dependencies:
  - github.com/test/repo/dep
variables:
  - name: MY_VAR
    default: hello
    sensitive: false
netbridge:
  - name: APP_PORT
    protocol: tcp
    default: 8080
lifecycle:
  install:
    - type: run
      command: "./install.sh"
      title: "Installing"
      timeout: 5m
  execute:
    - type: run
      command: "./start.sh"
  stop:
    - type: signal
      signal: SIGTERM
      timeout: 30s
  uninstall:
    - type: run
      command: "./uninstall.sh"
methods:
  validate:
    available_in: [ready]
    steps:
      - type: run
        command: "./validate.sh"
`)

var validQuiverV0 = []byte(`
schema: "quiver@v0"
name: "Test Quiver"
description: "A test quiver"
url: "https://example.com"
maintainers:
  - alice
tags:
  - test
`)

func TestNewTranslator(t *testing.T) {
	tr := NewTranslator()
	if tr == nil {
		t.Fatal("NewTranslator() returned nil")
	}
}

func TestTranslator_MultipleInstances(t *testing.T) {
	tr1 := NewTranslator()
	tr2 := NewTranslator()
	if tr1 == nil || tr2 == nil {
		t.Error("NewTranslator() returned nil")
	}
}

func TestTranslator_Arrow_Valid(t *testing.T) {
	tr := NewTranslator()
	raw, err := tr.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if raw.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", raw.Name)
	}
	if raw.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", raw.Version)
	}
	if raw.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", raw.Requirements.CpuCores)
	}
	if raw.Requirements.MemoryGB != 4 {
		t.Errorf("MemoryGB = %d, want 4", raw.Requirements.MemoryGB)
	}
	if len(raw.Requirements.OS) != 2 {
		t.Errorf("OS count = %d, want 2", len(raw.Requirements.OS))
	}
	if len(raw.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(raw.Variables))
	}
	if len(raw.Netbridge) != 1 {
		t.Errorf("Netbridge count = %d, want 1", len(raw.Netbridge))
	}
	if len(raw.Lifecycle.Install) != 1 {
		t.Errorf("Install steps = %d, want 1", len(raw.Lifecycle.Install))
	}
	if len(raw.Lifecycle.Execute) != 1 {
		t.Errorf("Execute steps = %d, want 1", len(raw.Lifecycle.Execute))
	}
	if len(raw.Lifecycle.Stop) != 1 {
		t.Errorf("Stop steps = %d, want 1", len(raw.Lifecycle.Stop))
	}
	if len(raw.Lifecycle.Uninstall) != 1 {
		t.Errorf("Uninstall steps = %d, want 1", len(raw.Lifecycle.Uninstall))
	}
	if _, ok := raw.Methods["validate"]; !ok {
		t.Error("expected 'validate' method")
	}
}

func TestTranslator_Arrow_Minimal(t *testing.T) {
	tr := NewTranslator()
	data := []byte("schema: \"arrow@v0\"\nname: min\nversion: 1.0.0\n")
	raw, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if raw.Name != "min" {
		t.Errorf("Name = %q, want min", raw.Name)
	}
}

func TestTranslator_Arrow_InvalidYAML(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Arrow([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTranslator_Arrow_MissingSchema(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Arrow([]byte("name: test\nversion: 1.0.0\n"))
	if err == nil {
		t.Error("expected error for missing schema field")
	}
}

func TestTranslator_Arrow_WrongSchemaType(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Arrow(validQuiverV0)
	if err == nil {
		t.Error("expected error for wrong schema type")
	}
}

func TestTranslator_Arrow_UnsupportedVersion(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Arrow([]byte("schema: \"arrow@v999\"\nname: x\nversion: 1.0.0\n"))
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestTranslator_Arrow_ValidationFailure(t *testing.T) {
	tr := NewTranslator()
	// Missing required 'version'
	_, err := tr.Arrow([]byte("schema: \"arrow@v0\"\nname: test\n"))
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
}

func TestTranslator_Quiver_Valid(t *testing.T) {
	tr := NewTranslator()
	raw, err := tr.Quiver(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if raw.Name != "Test Quiver" {
		t.Errorf("Name = %q, want 'Test Quiver'", raw.Name)
	}
	if raw.Description != "A test quiver" {
		t.Errorf("Description = %q, want 'A test quiver'", raw.Description)
	}
	if raw.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", raw.URL)
	}
	if len(raw.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d, want 1", len(raw.Maintainers))
	}
	if len(raw.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(raw.Tags))
	}
}

func TestTranslator_Quiver_InvalidYAML(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Quiver([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTranslator_Quiver_WrongSchemaType(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Quiver(validArrowV0)
	if err == nil {
		t.Error("expected error for wrong schema type")
	}
}

func TestTranslator_Quiver_UnsupportedVersion(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Quiver([]byte("schema: \"quiver@v999\"\nname: x\ndescription: y\n"))
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestTranslator_Quiver_ValidationFailure(t *testing.T) {
	tr := NewTranslator()
	// Missing required 'description'
	_, err := tr.Quiver([]byte("schema: \"quiver@v0\"\nname: test\n"))
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
}

func TestTranslator_ReadSchemaInfo_Arrow(t *testing.T) {
	tr := NewTranslator()
	info, err := tr.ReadSchemaInfo([]byte("schema: \"arrow@v0\"\nname: x\nversion: 1.0.0\n"))
	if err != nil {
		t.Fatalf("ReadSchemaInfo() error = %v", err)
	}
	if info.SchemaType != "arrow" {
		t.Errorf("SchemaType = %q, want arrow", info.SchemaType)
	}
	if info.Version != "v0" {
		t.Errorf("Version = %q, want v0", info.Version)
	}
	if info.ManifestKey != "arrow@v0" {
		t.Errorf("ManifestKey = %q, want arrow@v0", info.ManifestKey)
	}
}

func TestTranslator_ReadSchemaInfo_Quiver(t *testing.T) {
	tr := NewTranslator()
	info, err := tr.ReadSchemaInfo([]byte("schema: \"quiver@v0\"\nname: x\ndescription: y\n"))
	if err != nil {
		t.Fatalf("ReadSchemaInfo() error = %v", err)
	}
	if info.SchemaType != "quiver" {
		t.Errorf("SchemaType = %q, want quiver", info.SchemaType)
	}
}

func TestTranslator_ReadSchemaInfo_InvalidYAML(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.ReadSchemaInfo([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTranslator_ReadSchemaInfo_MissingSchema(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.ReadSchemaInfo([]byte("name: test\nversion: 1.0.0\n"))
	if err == nil {
		t.Error("expected error for missing schema field")
	}
}

// ─── Package-level convenience functions ─────────────────────────────────────

func TestArrow_PackageLevel_Valid(t *testing.T) {
	reader := NewTranslator()

	raw, err := reader.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if raw.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", raw.Name)
	}
}

func TestArrow_PackageLevel_Invalid(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Arrow([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestQuiver_PackageLevel_Valid(t *testing.T) {
	reader := NewTranslator()

	raw, err := reader.Quiver(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if raw.Name != "Test Quiver" {
		t.Errorf("Name = %q, want Test Quiver", raw.Name)
	}
}

func TestQuiver_PackageLevel_Invalid(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Quiver([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
