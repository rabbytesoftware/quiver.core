package v0_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow/v0"
)

func TestModule_Version(t *testing.T) {
	if v0.Default.Version() != "v0" {
		t.Errorf("Version() = %q, want v0", v0.Default.Version())
	}
}

func TestModule_GetSchema(t *testing.T) {
	schema, err := v0.Default.GetSchema()
	if err != nil {
		t.Fatalf("GetSchema() error = %v", err)
	}
	if len(schema) == 0 {
		t.Error("GetSchema() returned empty schema")
	}
}

var minimalArrowYAML = []byte(`
schema: "arrow@v0"
name: test-arrow
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
`)

func TestModule_Map_Minimal(t *testing.T) {
	result, err := v0.Default.Map(minimalArrowYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if result.Name != "test-arrow" {
		t.Errorf("Name = %q, want test-arrow", result.Name)
	}
	if result.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", result.Version)
	}
	if result.Requirements.CpuCores != 1 {
		t.Errorf("CpuCores = %d, want 1", result.Requirements.CpuCores)
	}
	if result.Requirements.MemoryGB != 1 {
		t.Errorf("MemoryGB = %d, want 1", result.Requirements.MemoryGB)
	}
	if result.Requirements.DiskGB != 1 {
		t.Errorf("DiskGB = %d, want 1", result.Requirements.DiskGB)
	}
}

var fullArrowYAML = []byte(`
schema: "arrow@v0"
name: cs2-server
description: CS2 Dedicated Server
version: 25.11.0
license: MIT
url: https://example.com
maintainers:
  - char2cs
tags:
  - gaming
credits:
  - name: Valve
    email: contact@valvesoftware.com
    url: https://valvesoftware.com
requirements:
  cpu_cores: 2
  ram_gb: 8
  disk_gb: 60
  os:
    - linux/amd64
    - linux/arm64
dependencies:
  - github.com/valvesoftware/quiver/steamcmd
variables:
  - name: CS2_PORT
    default: "27015"
    sensitive: false
    description: Game port number
    type: string
netbridge:
  - name: GAME_PORT
    protocol: tcp/udp
    default: 27015
    required: true
lifecycle:
  install:
    - type: run
      command: "./install.sh"
      title: "Installing CS2"
      timeout: 30m
      exit_on_failure: true
  execute:
    - type: run
      command: "./srcds_run -game csgo"
      title: "Starting CS2"
  stop:
    - type: signal
      signal: SIGTERM
      timeout: 30s
  uninstall:
    - type: run
      command: "./cleanup.sh"
      title: "Cleanup"
methods:
  update:
    available_in: [ready]
    steps:
      - type: run
        command: "./update.sh"
        title: "Updating CS2"
`)

func TestModule_Map_Full(t *testing.T) {
	result, err := v0.Default.Map(fullArrowYAML)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if result.Name != "cs2-server" {
		t.Errorf("Name = %q, want cs2-server", result.Name)
	}
	if result.Description != "CS2 Dedicated Server" {
		t.Errorf("Description = %q", result.Description)
	}
	if result.License != "MIT" {
		t.Errorf("License = %q", result.License)
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q", result.URL)
	}

	if len(result.Maintainers) != 1 || result.Maintainers[0] != "char2cs" {
		t.Errorf("Maintainers = %v", result.Maintainers)
	}

	if len(result.Tags) != 1 || result.Tags[0] != "gaming" {
		t.Errorf("Tags = %v", result.Tags)
	}

	if len(result.Credits) != 1 {
		t.Errorf("Credits count = %d, want 1", len(result.Credits))
	}
	if result.Credits[0].Name != "Valve" {
		t.Errorf("Credit name = %q", result.Credits[0].Name)
	}

	if result.Requirements.CpuCores != 2 {
		t.Errorf("CpuCores = %d, want 2", result.Requirements.CpuCores)
	}
	if result.Requirements.MemoryGB != 8 {
		t.Errorf("MemoryGB = %d, want 8", result.Requirements.MemoryGB)
	}
	if len(result.Requirements.OS) != 2 {
		t.Errorf("OS count = %d, want 2", len(result.Requirements.OS))
	}

	if len(result.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(result.Dependencies))
	}
	if result.Dependencies[0] != domain.Namespace("github.com/valvesoftware/quiver/steamcmd") {
		t.Errorf("Dependency = %q", result.Dependencies[0])
	}

	if len(result.Variables) != 1 {
		t.Errorf("Variables count = %d, want 1", len(result.Variables))
	}
	if result.Variables[0].Name != "CS2_PORT" {
		t.Errorf("Variable name = %q", result.Variables[0].Name)
	}

	if len(result.Netbridge) != 1 {
		t.Errorf("Netbridge count = %d, want 1", len(result.Netbridge))
	}
	if result.Netbridge[0].Name != "GAME_PORT" {
		t.Errorf("Port name = %q", result.Netbridge[0].Name)
	}
	if result.Netbridge[0].Default != 27015 {
		t.Errorf("Port default = %d, want 27015", result.Netbridge[0].Default)
	}

	if len(result.Lifecycle.Install) != 1 {
		t.Errorf("Install steps = %d, want 1", len(result.Lifecycle.Install))
	}
	if len(result.Lifecycle.Execute) != 1 {
		t.Errorf("Execute steps = %d, want 1", len(result.Lifecycle.Execute))
	}
	if len(result.Lifecycle.Stop) != 1 {
		t.Errorf("Stop steps = %d, want 1", len(result.Lifecycle.Stop))
	}
	if len(result.Lifecycle.Uninstall) != 1 {
		t.Errorf("Uninstall steps = %d, want 1", len(result.Lifecycle.Uninstall))
	}

	if _, ok := result.Methods["update"]; !ok {
		t.Error("expected 'update' method")
	}
	if len(result.Methods["update"].Steps) != 1 {
		t.Errorf("Update steps = %d, want 1", len(result.Methods["update"].Steps))
	}
}

func TestModule_Map_InvalidYAML(t *testing.T) {
	_, err := v0.Default.Map([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestModule_Map_MultipleDependencies(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
dependencies:
  - github.com/org/dep1
  - github.com/org/dep2
  - github.com/org/dep3
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Dependencies) != 3 {
		t.Errorf("Dependencies count = %d, want 3", len(result.Dependencies))
	}
}

func TestModule_Map_MultipleStepTypes(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: multi-step
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: run
      command: "echo install"
    - type: fetch
      url: "http://example.com/file"
      to: "/tmp/file"
    - type: signal
      signal: SIGUSR1
    - type: dependencies
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Lifecycle.Install) != 4 {
		t.Errorf("Install steps = %d, want 4", len(result.Lifecycle.Install))
	}
}

func TestModule_Map_OverrideableStringFields(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: overrideable-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: run
      command: "./install.sh"
      timeout: "5m"
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	step := result.Lifecycle.Install[0]
	if step.Type() != "run" {
		t.Errorf("Step type = %v, want run", step.Type())
	}
}

func TestModule_Map_MethodAvailableStates(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: method-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
methods:
  validate:
    available_in: [ready, running]
    steps:
      - type: run
        command: "./validate.sh"
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Methods) != 1 {
		t.Errorf("Methods count = %d, want 1", len(result.Methods))
	}
	method := result.Methods["validate"]
	if len(method.AvailableIn) != 2 {
		t.Errorf("Available states = %d, want 2", len(method.AvailableIn))
	}
}

func TestModule_Map_EmptyOptionalFields(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: empty-optional
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Maintainers) != 0 {
		t.Errorf("Maintainers should be empty")
	}
	if len(result.Tags) != 0 {
		t.Errorf("Tags should be empty")
	}
	if len(result.Credits) != 0 {
		t.Errorf("Credits should be empty")
	}
	if len(result.Dependencies) != 0 {
		t.Errorf("Dependencies should be empty")
	}
	if len(result.Variables) != 0 {
		t.Errorf("Variables should be empty")
	}
	if len(result.Netbridge) != 0 {
		t.Errorf("Netbridge should be empty")
	}
}

func TestModule_Map_ProtocolConversion(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: protocol-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
netbridge:
  - name: TEST_PORT
    protocol: tcp
    default: 8080
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	port := result.Netbridge[0]
	if port.Protocol != netbridge.Protocol("tcp") {
		t.Errorf("Protocol = %v, want tcp", port.Protocol)
	}
}

func TestModule_Map_VariableTypeConversion(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: var-type-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
variables:
  - name: MY_VAR
    type: select
    values: [a, b, c]
    default: a
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	v := result.Variables[0]
	if v.Type != domain.VariableType("select") {
		t.Errorf("Type = %v, want select", v.Type)
	}
	if len(v.Values) != 3 {
		t.Errorf("Values count = %d, want 3", len(v.Values))
	}
}

func TestModule_Map_UnknownStepType(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: bad-step-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: unknown_type
      title: "Bad step"
`)
	_, err := v0.Default.Map(yamlData)
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
}

func TestModule_Map_InvalidStepInMethod(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: bad-method-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
methods:
  test_method:
    available_in: [ready]
    steps:
      - type: invalid
        title: "Bad step"
`)
	_, err := v0.Default.Map(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in method")
	}
}

func TestModule_Map_MultipleCredits(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: credits-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
credits:
  - name: Alice
    email: alice@example.com
    url: https://alice.example.com
  - name: Bob
    email: bob@example.com
    url: https://bob.example.com
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Credits) != 2 {
		t.Errorf("Credits count = %d, want 2", len(result.Credits))
	}
	if result.Credits[0].Name != "Alice" {
		t.Errorf("First credit name = %q, want Alice", result.Credits[0].Name)
	}
	if result.Credits[1].Name != "Bob" {
		t.Errorf("Second credit name = %q, want Bob", result.Credits[1].Name)
	}
}

func TestModule_Map_AllStepTypes(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: all-steps-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: run
      command: "echo install"
      timeout: 5m
    - type: fetch
      url: "https://example.com/file"
      to: "/tmp/file"
      timeout: 10m
    - type: signal
      signal: SIGUSR1
      timeout: 30s
    - type: dependencies
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Lifecycle.Install) != 4 {
		t.Errorf("Install steps = %d, want 4", len(result.Lifecycle.Install))
	}
}

func TestModule_Map_CompleteLifecycle(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: full-lifecycle
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: run
      command: "install"
  execute:
    - type: run
      command: "run"
  stop:
    - type: signal
      signal: SIGTERM
  uninstall:
    - type: run
      command: "cleanup"
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(result.Lifecycle.Install) != 1 {
		t.Error("install missing")
	}
	if len(result.Lifecycle.Execute) != 1 {
		t.Error("execute missing")
	}
	if len(result.Lifecycle.Stop) != 1 {
		t.Error("stop missing")
	}
	if len(result.Lifecycle.Uninstall) != 1 {
		t.Error("uninstall missing")
	}
}

func TestModule_Map_RunStepExitOnFailure(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: exit-test
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  install:
    - type: run
      command: "test"
      exit_on_failure: true
`)
	result, err := v0.Default.Map(yamlData)
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	steps := result.Lifecycle.Install
	if len(steps) == 0 {
		t.Fatal("no install steps")
	}
}

func TestModule_Map_InvalidExecuteStep(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: bad-execute
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  execute:
    - type: invalid_type
      command: "test"
`)
	_, err := v0.Default.Map(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid execute step type")
	}
}

func TestModule_Map_InvalidStopStep(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: bad-stop
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  stop:
    - type: badtype
`)
	_, err := v0.Default.Map(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid stop step type")
	}
}

func TestModule_Map_InvalidUninstallStep(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: bad-uninstall
version: 1.0.0
requirements:
  cpu_cores: 1
  ram_gb: 1
  disk_gb: 1
lifecycle:
  uninstall:
    - type: notavalidsteptype
`)
	_, err := v0.Default.Map(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid uninstall step type")
	}
}
