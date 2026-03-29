package translator

import (
	"fmt"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/schemas"
)

var validArrowV0 = []byte(`
schema: "arrow@v0"
metadata:
  name: test-arrow
  description: A test arrow
  version: 1.0.0
  license: MIT
  quiver: github.com/test/quiver
  credits:
    - name: Alice
      email: alice@example.com
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
media:
  icon: "https://example.com/icon.png"
arrows:
  - github.com/test/repo
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
	if raw.Metadata.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", raw.Metadata.Name)
	}
	if raw.Metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", raw.Metadata.Version)
	}
	if raw.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", raw.Requirements.CpuCores)
	}
	if raw.Requirements.RamGB != 4 {
		t.Errorf("RamGB = %d, want 4", raw.Requirements.RamGB)
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
	data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: min\n  version: 1.0.0\n")
	raw, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if raw.Metadata.Name != "min" {
		t.Errorf("Name = %q, want min", raw.Metadata.Name)
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
	_, err := tr.Arrow([]byte("metadata:\n  name: test"))
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
	_, err := tr.Arrow([]byte("schema: \"arrow@v999\"\nmetadata:\n  name: x\n  version: 1.0.0\n"))
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestTranslator_Arrow_ValidationFailure(t *testing.T) {
	tr := NewTranslator()
	// Missing required 'metadata.version'
	_, err := tr.Arrow([]byte("schema: \"arrow@v0\"\nmetadata:\n  name: test\n"))
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
}

func TestTranslator_Arrow_Credits(t *testing.T) {
	tr := NewTranslator()
	raw, err := tr.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if len(raw.Metadata.Credits) != 1 {
		t.Errorf("Credits count = %d, want 1", len(raw.Metadata.Credits))
	}
	if raw.Metadata.Credits[0].Name != "Alice" {
		t.Errorf("Credit name = %q, want Alice", raw.Metadata.Credits[0].Name)
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
	if raw.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Media.Icon = %q, want https://example.com/icon.png", raw.Media.Icon)
	}
	if len(raw.Arrows) != 1 {
		t.Errorf("Arrows count = %d, want 1", len(raw.Arrows))
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
	info, err := tr.ReadSchemaInfo([]byte("schema: \"arrow@v0\"\nmetadata:\n  name: x\n"))
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
	_, err := tr.ReadSchemaInfo([]byte("metadata:\n  name: test\n"))
	if err == nil {
		t.Error("expected error for missing schema field")
	}
}

func TestTranslator_Arrow_StepTypes(t *testing.T) {
	tr := NewTranslator()
	data := []byte(`
schema: "arrow@v0"
metadata:
  name: steps-test
  version: 1.0.0
lifecycle:
  install:
    - type: run
      command: "./setup.sh"
      title: "Setup"
      timeout: 10m
      exit_on_failure: true
    - type: fetch
      url: https://example.com/file.bin
      to: ./file.bin
      title: "Downloading"
    - type: signal
      signal: SIGUSR1
      timeout: 5s
    - type: dependencies
      title: "Installing dependencies"
`)
	raw, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if len(raw.Lifecycle.Install) != 4 {
		t.Errorf("Install steps = %d, want 4", len(raw.Lifecycle.Install))
	}
	if raw.Lifecycle.Install[0].Type != "run" {
		t.Errorf("Step[0].Type = %q, want run", raw.Lifecycle.Install[0].Type)
	}
	if raw.Lifecycle.Install[1].Type != "fetch" {
		t.Errorf("Step[1].Type = %q, want fetch", raw.Lifecycle.Install[1].Type)
	}
	if raw.Lifecycle.Install[2].Type != "signal" {
		t.Errorf("Step[2].Type = %q, want signal", raw.Lifecycle.Install[2].Type)
	}
}

// ─── Package-level convenience functions ─────────────────────────────────────

func TestArrow_PackageLevel_Valid(t *testing.T) {
	raw, err := Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if raw.Metadata.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", raw.Metadata.Name)
	}
}

func TestArrow_PackageLevel_Invalid(t *testing.T) {
	_, err := Arrow([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestQuiver_PackageLevel_Valid(t *testing.T) {
	raw, err := Quiver(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if raw.Name != "Test Quiver" {
		t.Errorf("Name = %q, want Test Quiver", raw.Name)
	}
}

func TestQuiver_PackageLevel_Invalid(t *testing.T) {
	_, err := Quiver([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ─── validateYAML error path ──────────────────────────────────────────────────

func TestValidateYAML_InvalidYAML(t *testing.T) {
	err := validateYAML([]byte(`{}`), []byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ─── GetSchema() error path ───────────────────────────────────────────────────

type failSchemaMapper struct{}

func (f *failSchemaMapper) GetSchema() ([]byte, error) {
	return nil, fmt.Errorf("schema load failure")
}

func (f *failSchemaMapper) Map(_ []byte) (*models.RawArrow, error) {
	return nil, nil
}

func TestReadManifest_GetSchemaError(t *testing.T) {
	data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: x\n  version: 1.0.0\n")
	getMapper := func(_ string) (schemas.Mapper[models.RawArrow], error) {
		return &failSchemaMapper{}, nil
	}
	_, err := readManifest(data, "arrow", getMapper)
	if err == nil {
		t.Error("expected error when GetSchema() fails")
	}
}

func TestTranslator_Arrow_StepOverrides(t *testing.T) {
	tr := NewTranslator()
	data := []byte(`
schema: "arrow@v0"
metadata:
  name: overrides-test
  version: 1.0.0
requirements:
  os:
    - linux/amd64
    - windows/amd64
lifecycle:
  install:
    - type: run
      command: "./install.sh"
      overrides:
        windows/amd64:
          command: ".\\install.bat"
          title: "Windows install"
`)
	raw, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	step := raw.Lifecycle.Install[0]
	if override, ok := step.Overrides["windows/amd64"]; !ok {
		t.Error("expected windows/amd64 override")
	} else if override.Command != `.\install.bat` {
		t.Errorf("override command = %q, want .\\install.bat", override.Command)
	}
}
