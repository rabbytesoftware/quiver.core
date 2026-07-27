package translator

import (
	"testing"
)

// ─── Manifest parsing tests (from manifest_test.go) ──────────────────────────

var validArrowV0 = []byte(`
schema: "arrow@v0"
metadata:
  name: test-arrow
  description: A test arrow
  license: MIT
  maintainers:
    - name: alice
  tags:
    - test
variables:
  - name: MY_VAR
    default: hello
    sensitive: false
netbridge:
  - name: APP_PORT
    protocol: tcp
    default: 8080
targets:
  "*":
    requirements:
      cpu_cores: 2
      ram_gb: 4
      disk_gb: 10
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
schema: "collection@v0"
metadata:
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
	mod, err := tr.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if mod.Manifest.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", mod.Manifest.Name)
	}
	target, ok := mod.Precompiled["*"]
	if !ok {
		t.Fatal("Precompiled[\"*\"] does not exist")
	}
	if target.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", target.Requirements.CpuCores)
	}
	if target.Requirements.MemoryGB != 4 {
		t.Errorf("MemoryGB = %d, want 4", target.Requirements.MemoryGB)
	}
	if len(mod.Manifest.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(mod.Manifest.Variables))
	}
	if len(mod.Manifest.Netbridge) != 1 {
		t.Errorf("Netbridge count = %d, want 1", len(mod.Manifest.Netbridge))
	}
	if len(target.Lifecycle.Install) != 1 {
		t.Errorf("Install steps = %d, want 1", len(target.Lifecycle.Install))
	}
	if len(target.Lifecycle.Execute) != 1 {
		t.Errorf("Execute steps = %d, want 1", len(target.Lifecycle.Execute))
	}
	if len(target.Lifecycle.Stop) != 1 {
		t.Errorf("Stop steps = %d, want 1", len(target.Lifecycle.Stop))
	}
	if len(target.Lifecycle.Uninstall) != 1 {
		t.Errorf("Uninstall steps = %d, want 1", len(target.Lifecycle.Uninstall))
	}
	if _, ok := target.Methods["validate"]; !ok {
		t.Error("expected 'validate' method")
	}
}

func TestTranslator_Arrow_Minimal(t *testing.T) {
	tr := NewTranslator()
	data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: min\ntargets:\n  \"*\":\n    lifecycle:\n      install:\n        - type: run\n          command: echo ok\n")
	mod, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if mod.Manifest.Name != "min" {
		t.Errorf("Name = %q, want min", mod.Manifest.Name)
	}
}

// TestTranslator_Arrow_MetadataVersionTolerated pins tolerate-and-discard: the
// schema still lists `version` so the key does not trip additionalProperties,
// but no Go type models it, so an arrow authored with one still translates and
// the value has nowhere to land — an arrow's version is the ref it resolves at.
func TestTranslator_Arrow_MetadataVersionTolerated(t *testing.T) {
	tr := NewTranslator()
	data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: min\n  version: 1.0.0\ntargets:\n  \"*\":\n    lifecycle:\n      install:\n        - type: run\n          command: echo ok\n")

	mod, err := tr.Arrow(data)
	if err != nil {
		t.Fatalf("Arrow() error = %v, want a clean parse for an authored metadata.version", err)
	}
	if mod.Manifest.Name != "min" {
		t.Errorf("Name = %q, want min", mod.Manifest.Name)
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
	// Missing required 'metadata' and 'targets'
	_, err := tr.Arrow([]byte("schema: \"arrow@v0\"\nname: test\n"))
	if err == nil {
		t.Error("expected validation error for missing required field")
	}
}

func TestTranslator_Quiver_Valid(t *testing.T) {
	tr := NewTranslator()
	mod, err := tr.Collection(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if mod.Manifest.Meta.Name != "Test Quiver" {
		t.Errorf("Name = %q, want 'Test Quiver'", mod.Manifest.Meta.Name)
	}
	if mod.Manifest.Meta.Description != "A test quiver" {
		t.Errorf("Description = %q, want 'A test quiver'", mod.Manifest.Meta.Description)
	}
	if mod.Manifest.Meta.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", mod.Manifest.Meta.URL)
	}
	if len(mod.Manifest.Meta.Maintainers) != 1 {
		t.Errorf("Maintainers count = %d, want 1", len(mod.Manifest.Meta.Maintainers))
	}
	if len(mod.Manifest.Meta.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(mod.Manifest.Meta.Tags))
	}
}

func TestTranslator_Quiver_InvalidYAML(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Collection([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTranslator_Quiver_WrongSchemaType(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Collection(validArrowV0)
	if err == nil {
		t.Error("expected error for wrong schema type")
	}
}

func TestTranslator_Quiver_UnsupportedVersion(t *testing.T) {
	tr := NewTranslator()
	_, err := tr.Collection([]byte("schema: \"collection@v999\"\nmetadata:\n  name: x\n  description: y\n"))
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestTranslator_Quiver_ValidationFailure(t *testing.T) {
	tr := NewTranslator()
	// Missing required 'description' inside metadata
	_, err := tr.Collection([]byte("schema: \"collection@v0\"\nmetadata:\n  name: test\n"))
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
	info, err := tr.ReadSchemaInfo([]byte("schema: \"collection@v0\"\nmetadata:\n  name: x\n  description: y\n"))
	if err != nil {
		t.Fatalf("ReadSchemaInfo() error = %v", err)
	}
	if info.SchemaType != "collection" {
		t.Errorf("SchemaType = %q, want collection", info.SchemaType)
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

	mod, err := reader.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if mod.Manifest.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", mod.Manifest.Name)
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

	mod, err := reader.Collection(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if mod.Manifest.Meta.Name != "Test Quiver" {
		t.Errorf("Name = %q, want Test Quiver", mod.Manifest.Meta.Name)
	}
}

func TestQuiver_PackageLevel_Invalid(t *testing.T) {
	reader := NewTranslator()

	_, err := reader.Collection([]byte("invalid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// ─── Edge case tests ────────────────────────────────────────────────────────

func TestTranslator_Arrow_EmptySchema(t *testing.T) {
	tr := NewTranslator()
	// Empty schema field
	data := []byte("schema: \"\"\nname: empty\nversion: 1.0.0\n")
	_, err := tr.Arrow(data)
	if err == nil {
		t.Error("expected error for empty schema field")
	}
}

func TestTranslator_Arrow_MalformedSchemaVersion(t *testing.T) {
	tr := NewTranslator()
	// Malformed schema: missing version part after @
	data := []byte("schema: \"arrow@\"\nname: malformed\nversion: 1.0.0\n")
	_, err := tr.Arrow(data)
	if err == nil {
		t.Error("expected error for malformed schema version")
	}
}

func TestTranslator_Arrow_MultipleValidArrows(t *testing.T) {
	tr := NewTranslator()
	data1 := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: arrow1\ntargets:\n  \"*\":\n    lifecycle:\n      install:\n        - type: run\n          command: echo ok\n")
	data2 := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: arrow2\ntargets:\n  \"*\":\n    lifecycle:\n      install:\n        - type: run\n          command: echo ok\n")

	mod1, err := tr.Arrow(data1)
	if err != nil {
		t.Fatalf("Arrow() for arrow1 error = %v", err)
	}
	if mod1.Manifest.Name != "arrow1" {
		t.Errorf("Name = %q, want arrow1", mod1.Manifest.Name)
	}

	mod2, err := tr.Arrow(data2)
	if err != nil {
		t.Fatalf("Arrow() for arrow2 error = %v", err)
	}
	if mod2.Manifest.Name != "arrow2" {
		t.Errorf("Name = %q, want arrow2", mod2.Manifest.Name)
	}
}

func TestTranslator_Arrow_SchemaValidationFailure(t *testing.T) {
	tr := NewTranslator()
	// Invalid type for required field
	data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: 123\ntargets:\n  \"*\":\n    lifecycle:\n      install:\n        - type: run\n          command: echo ok\n")
	_, err := tr.Arrow(data)
	if err == nil {
		t.Error("expected validation error for invalid field type")
	}
}

func TestTranslator_Quiver_SchemaGetFailure(t *testing.T) {
	tr := NewTranslator()
	// Valid quiver YAML should succeed
	mod, err := tr.Collection(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if mod.Manifest.Meta.Name == "" {
		t.Error("expected non-empty manifest name")
	}
}

func TestReadManifest_InvalidSchema(t *testing.T) {
	tr := NewTranslator()
	// Call with invalid YAML should propagate error
	_, err := tr.Collection([]byte("not-valid: yaml: [[["))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestTranslator_Arrow_ParseError(t *testing.T) {
	tr := NewTranslator()
	// Valid YAML and schema but parse fails
	invalidData := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: test\nversion: 1.0.0\ninvalid_field_that_breaks_parsing: oops\n")
	_, err := tr.Arrow(invalidData)
	if err == nil {
		t.Error("expected error for unparseable arrow data")
	}
}

func TestTranslator_Quiver_InvalidManifest(t *testing.T) {
	tr := NewTranslator()
	// Additional property not allowed by schema
	data := []byte("schema: \"collection@v0\"\nmetadata:\n  name: test\n  description: test\nunknown_field: oops\n")
	_, err := tr.Collection(data)
	if err == nil {
		t.Error("expected error for additional property in quiver")
	}
}

// ─── Markdown codeblock extraction tests ────────────────────────────────────

func TestArrow_MarkdownInput(t *testing.T) {
	wrappedMD := append([]byte("# Title\n\nSome description.\n\n```arrow\n"), validArrowV0...)
	wrappedMD = append(wrappedMD, []byte("\n```\n")...)

	tr := NewTranslator()
	module, err := tr.Arrow(wrappedMD)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if module.Manifest == nil {
		t.Fatal("expected non-nil Manifest")
	}
}

func TestArrow_MarkdownInput_NoCodeblock_FallsBackToYAML(t *testing.T) {
	tr := NewTranslator()
	module, err := tr.Arrow(validArrowV0)
	if err != nil {
		t.Fatalf("Arrow() error = %v", err)
	}
	if module.Manifest == nil {
		t.Fatal("expected non-nil Manifest")
	}
}

func TestArrow_MarkdownInput_CRLF(t *testing.T) {
	// Build the same markdown but with \r\n line endings
	unix := append([]byte("# Title\r\n\r\n```arrow\r\n"), validArrowV0...)
	crlf := append(unix, []byte("\r\n```\r\n")...)

	tr := NewTranslator()
	module, err := tr.Arrow(crlf)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if module.Manifest == nil {
		t.Fatal("expected non-nil manifest")
	}
}

func TestArrow_MarkdownInput_EmptyCodeblock_ReturnsError(t *testing.T) {
	tr := NewTranslator()
	md := []byte("# Title\n\n```arrow\n```\n")
	_, err := tr.Arrow(md)
	if err == nil {
		t.Error("expected error for empty arrow codeblock")
	}
}

func TestQuiver_MarkdownInput(t *testing.T) {
	md := []byte("# My Collection\n\nSome description.\n\n```collection\n" + string(validQuiverV0) + "\n```\n")
	tr := NewTranslator()
	mod, err := tr.Collection(md)
	if err != nil {
		t.Fatalf("Quiver() from QUIVER.md error = %v", err)
	}
	if mod.Manifest.Meta.Name != "Test Quiver" {
		t.Errorf("Name = %q, want 'Test Quiver'", mod.Manifest.Meta.Name)
	}
}

func TestQuiver_MarkdownInput_NoCodeblock_FallsBackToYAML(t *testing.T) {
	tr := NewTranslator()
	mod, err := tr.Collection(validQuiverV0)
	if err != nil {
		t.Fatalf("Quiver() error = %v", err)
	}
	if mod.Manifest.Meta.Name != "Test Quiver" {
		t.Errorf("Name = %q, want 'Test Quiver'", mod.Manifest.Meta.Name)
	}
}
