package v0_test

import (
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

func TestModule_Version(t *testing.T) {
	if v0.New().Version() != "v0" {
		t.Errorf("Version() = %q, want v0", v0.New().Version())
	}
}

func TestModule_GetSchema(t *testing.T) {
	schema := v0.New().Schema()
	if len(schema) == 0 {
		t.Error("Schema() returned empty schema")
	}
}

// TestMap_HappyPath: full manifest with all top-level sections parses correctly.
func TestMap_HappyPath(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: cs2-server
  description: CS2 Dedicated Server
  license: MIT
  url: https://example.com
  maintainers:
    - name: char2cs
      email: char@example.com
      url: https://char2cs.dev
  tags:
    - gaming
  credits:
    - name: Valve
      email: contact@valvesoftware.com
      url: https://valvesoftware.com
variables:
  - name: MY_VAR
    type: select
    values: [a, b, c]
    default: a
netbridge:
  - name: GAME_PORT
    protocol: tcp
    default: 27015
    required: true
targets:
  "*":
    requirements:
      cpu_cores: 4
      ram_gb: 8
      disk_gb: 60
    exports:
      BIN_PATH: "/usr/local/bin/mytool"
    lifecycle:
      install:
        - type: run
          command: "install"
      update:
        - type: run
          command: "update"
      execute:
        - type: run
          command: "./server"
      stop:
        - type: signal
          signal: SIGTERM
      uninstall:
        - type: run
          command: "cleanup"
    methods:
      validate:
        available_in: [ready, running]
        steps:
          - type: run
            command: "./validate.sh"
  linux/amd64:
    lifecycle:
      install:
        - type: run
          command: "echo linux"
`)
	result, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// metadata
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
	if len(result.Maintainers) != 1 || result.Maintainers[0].Name != "char2cs" {
		t.Errorf("Maintainers = %v", result.Maintainers)
	}
	if len(result.Tags) != 1 || result.Tags[0] != "gaming" {
		t.Errorf("Tags = %v", result.Tags)
	}
	if len(result.Credits) != 1 || result.Credits[0].Name != "Valve" {
		t.Errorf("Credits = %v", result.Credits)
	}

	// variables
	if len(result.Variables) != 1 {
		t.Fatalf("Variables count = %d, want 1", len(result.Variables))
	}
	v := result.Variables[0]
	if v.Name != "MY_VAR" || v.Type != domain.VariableType("select") || len(v.Values) != 3 {
		t.Errorf("Variable = %+v", v)
	}

	// netbridge
	if len(result.Netbridge) != 1 {
		t.Fatalf("Netbridge count = %d, want 1", len(result.Netbridge))
	}
	port := result.Netbridge[0]
	if port.Name != "GAME_PORT" || port.Default != 27015 || port.Protocol != netbridge.Protocol("tcp") {
		t.Errorf("Netbridge port = %+v", port)
	}

	// targets
	if len(precompiled) != 2 {
		t.Errorf("precompiled count = %d, want 2", len(precompiled))
	}
	wildcard, ok := precompiled["*"]
	if !ok {
		t.Fatal("missing target \"*\"")
	}
	if _, ok := precompiled["linux/amd64"]; !ok {
		t.Fatal("missing target \"linux/amd64\"")
	}

	// requirements
	req := wildcard.Requirements
	if req.CpuCores != 4 || req.MemoryGB != 8 || req.DiskGB != 60 {
		t.Errorf("Requirements = %+v", req)
	}

	// exports
	if wildcard.Exports["BIN_PATH"].Default != "/usr/local/bin/mytool" {
		t.Errorf("BIN_PATH = %q", wildcard.Exports["BIN_PATH"].Default)
	}

	// lifecycle phases
	lc := wildcard.Lifecycle
	if len(lc.Install) != 1 || len(lc.Update) != 1 || len(lc.Execute) != 1 || len(lc.Stop) != 1 || len(lc.Uninstall) != 1 {
		t.Errorf("Lifecycle phase counts: install=%d update=%d execute=%d stop=%d uninstall=%d",
			len(lc.Install), len(lc.Update), len(lc.Execute), len(lc.Stop), len(lc.Uninstall))
	}

	// methods
	if len(wildcard.Methods) != 1 {
		t.Errorf("Methods count = %d, want 1", len(wildcard.Methods))
	}
	if len(wildcard.Methods["validate"].AvailableIn) != 2 {
		t.Errorf("validate.AvailableIn = %v", wildcard.Methods["validate"].AvailableIn)
	}
}

// TestMap_TargetBasePreserved: base field on a target survives parsing.
func TestMap_TargetBasePreserved(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: base-test
targets:
  linux/amd64:
    base: _common
    lifecycle:
      install:
        - type: run
          command: "echo linux"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if precompiled["linux/amd64"].Base != "_common" {
		t.Errorf("Base = %q, want _common", precompiled["linux/amd64"].Base)
	}
}

// TestMap_Tools: tools strings parse to versioned Namespace; missing version yields empty ref.
func TestMap_Tools(t *testing.T) {
	tests := []struct {
		name          string
		tool          string
		wantNamespace domain.Namespace
		wantRef       string
	}{
		{
			name:          "with version",
			tool:          "github.com/foo/bar@v1.2.3",
			wantNamespace: "github.com/foo/bar",
			wantRef:       "v1.2.3",
		},
		{
			name:          "without version",
			tool:          "github.com/org/tool",
			wantNamespace: "github.com/org/tool",
			wantRef:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: tools-test
targets:
  "*":
    tools:
      - ` + tt.tool + `
    lifecycle:
      install:
        - type: run
          command: "echo ok"
`)
			_, precompiled, err := v0.New().Parse(yamlData)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			ns := precompiled["*"].Tools[0]
			if ns.BareNamespace() != tt.wantNamespace {
				t.Errorf("BareNamespace = %q, want %q", ns.BareNamespace(), tt.wantNamespace)
			}
			if ns.Ref() != tt.wantRef {
				t.Errorf("Ref = %q, want %q", ns.Ref(), tt.wantRef)
			}
		})
	}
}

// TestMap_PreRefactorShapeReturnsError: old flat-shape manifest returns error containing "pre-refactor".
func TestMap_PreRefactorShapeReturnsError(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
name: old-style
version: 1.0.0
lifecycle:
  install:
    - type: run
      command: "echo old"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for pre-refactor manifest shape")
	}
	if !strings.Contains(err.Error(), "pre-refactor") {
		t.Errorf("error = %q, want it to contain \"pre-refactor\"", err.Error())
	}
}

func TestMap_InvalidYAML(t *testing.T) {
	_, _, err := v0.New().Parse([]byte("not: valid: yaml: :"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// An Overrideable map is typed per field, so a value of the wrong shape must
// surface the decode error rather than silently yielding a zero override.
func TestMap_OverrideableMapWithNonScalarValueReturnsError(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-overrideable
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command:
            default: ["not", "a", "string"]
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for non-scalar Overrideable map value")
	}
}

func TestMap_UnknownStepType(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-step-test
targets:
  "*":
    lifecycle:
      install:
        - type: unknown_type
          title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
}

func TestMap_DependenciesStepForbidden(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: deps-step-test
targets:
  "*":
    lifecycle:
      install:
        - type: dependencies
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for dependencies step type in manifest")
	}
	if !strings.Contains(err.Error(), "synthetic") {
		t.Errorf("error = %q, want it to contain \"synthetic\"", err.Error())
	}
}

func TestMap_InvalidStepInMethod(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-method-test
targets:
  "*":
    methods:
      test_method:
        available_in: [ready]
        steps:
          - type: invalid
            title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in method")
	}
}

// TestMap_StepExitOnFailure: default is true; explicit false allows continue.
func TestMap_StepExitOnFailure(t *testing.T) {
	tests := []struct {
		name           string
		yaml           string
		wantExitOnFail bool
	}{
		{
			name: "default true",
			yaml: `
schema: "arrow@v0"
metadata:
  name: default-exit-test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "do-something"
`,
			wantExitOnFail: true,
		},
		{
			name: "explicit false",
			yaml: `
schema: "arrow@v0"
metadata:
  name: continue-on-fail-test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "best-effort"
          exit_on_failure: false
`,
			wantExitOnFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, precompiled, err := v0.New().Parse([]byte(tt.yaml))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			steps := precompiled["*"].Lifecycle.Install
			if len(steps) == 0 {
				t.Fatal("no install steps")
			}
			if steps[0].ExitOnFailure() != tt.wantExitOnFail {
				t.Errorf("ExitOnFailure() = %v, want %v", steps[0].ExitOnFailure(), tt.wantExitOnFail)
			}
		})
	}
}

// TestMap_OverrideableMapFields: overrideable fields (map form) parse to Default + OSArch entries.
func TestMap_OverrideableMapFields(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: overrideable-map-test
targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          title: "Download binary"
          url:
            default: "https://example.com/binary.tar.gz"
            linux/amd64: "https://example.com/linux-amd64.tar.gz"
            darwin/arm64: "https://example.com/darwin-arm64.tar.gz"
          to:
            default: "${WORKDIR}/binary.tar.gz"
            linux/amd64: "${WORKDIR}/linux-amd64.tar.gz"
          timeout: 5m
      stop:
        - type: signal
          signal:
            default: graceful
            linux/amd64: SIGTERM
      uninstall:
        - type: run
          title: "Cleanup"
          command:
            default: "rm -f binary.tar.gz"
            linux/amd64: "rm -f linux-amd64.tar.gz"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	lc := precompiled["*"].Lifecycle

	fetchStep, ok := lc.Install[0].(step.FetchStep)
	if !ok {
		t.Fatal("expected FetchStep")
	}
	if fetchStep.URL.Default != "https://example.com/binary.tar.gz" {
		t.Errorf("URL default = %q", fetchStep.URL.Default)
	}
	if fetchStep.URL.OSArch["linux/amd64"] != "https://example.com/linux-amd64.tar.gz" {
		t.Errorf("URL linux/amd64 = %q", fetchStep.URL.OSArch["linux/amd64"])
	}
	if fetchStep.URL.OSArch["darwin/arm64"] != "https://example.com/darwin-arm64.tar.gz" {
		t.Errorf("URL darwin/arm64 = %q", fetchStep.URL.OSArch["darwin/arm64"])
	}
	if fetchStep.To.Default != "${WORKDIR}/binary.tar.gz" {
		t.Errorf("To default = %q", fetchStep.To.Default)
	}

	sig, ok := lc.Stop[0].(step.SignalStep)
	if !ok {
		t.Fatalf("stop[0] is %T, want SignalStep", lc.Stop[0])
	}
	if sig.Signal.Default != "graceful" {
		t.Errorf("Signal.Default = %q, want graceful", sig.Signal.Default)
	}
	if sig.Signal.OSArch["linux/amd64"] != "SIGTERM" {
		t.Errorf("Signal.OSArch[linux/amd64] = %q, want SIGTERM", sig.Signal.OSArch["linux/amd64"])
	}

	runStep, ok := lc.Uninstall[0].(step.RunStep)
	if !ok {
		t.Fatal("expected RunStep")
	}
	if runStep.Command.Default != "rm -f binary.tar.gz" {
		t.Errorf("Command default = %q", runStep.Command.Default)
	}
	if runStep.Command.OSArch["linux/amd64"] != "rm -f linux-amd64.tar.gz" {
		t.Errorf("Command linux/amd64 = %q", runStep.Command.OSArch["linux/amd64"])
	}
}

func TestMap_InvalidStepInUpdate(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-update-test
targets:
  "*":
    lifecycle:
      update:
        - type: invalid_type
          title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in update lifecycle")
	}
}

func TestMap_InvalidStepInExecute(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-execute-test
targets:
  "*":
    lifecycle:
      execute:
        - type: invalid_type
          title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in execute lifecycle")
	}
}

func TestMap_InvalidStepInStop(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-stop-test
targets:
  "*":
    lifecycle:
      stop:
        - type: invalid_type
          title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in stop lifecycle")
	}
}

func TestMap_InvalidStepInUninstall(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: bad-uninstall-test
targets:
  "*":
    lifecycle:
      uninstall:
        - type: invalid_type
          title: "Bad step"
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid step in uninstall lifecycle")
	}
}

// TestMap_MediaMappedToAggregate: icon and banner from metadata.media survive to domain.ArrowMeta.
func TestMap_MediaMappedToAggregate(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: media-test
  media:
    icon: https://example.com/icon.png
    banner: https://example.com/banner.png
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "echo ok"
`)
	result, _, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Media.Icon != "https://example.com/icon.png" {
		t.Errorf("Media.Icon = %q, want https://example.com/icon.png", result.Media.Icon)
	}
	if result.Media.Banner != "https://example.com/banner.png" {
		t.Errorf("Media.Banner = %q, want https://example.com/banner.png", result.Media.Banner)
	}
}

// TestMap_MediaAbsentYieldsZeroValues: missing media section yields empty strings.
func TestMap_MediaAbsentYieldsZeroValues(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: no-media-test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "echo ok"
`)
	result, _, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if result.Media.Icon != "" || result.Media.Banner != "" {
		t.Errorf("expected zero Media, got Icon=%q Banner=%q", result.Media.Icon, result.Media.Banner)
	}
}

func TestMap_OverrideableYAML_SequenceNodeError(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: seq-node-test
targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          url:
            - item1
            - item2
          to: ./file
`)
	_, _, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for sequence node in overrideable field")
	}
}
