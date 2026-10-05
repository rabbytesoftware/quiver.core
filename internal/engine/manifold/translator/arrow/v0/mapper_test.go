package v0_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

// validateAgainstSchema mirrors translator.validateYAML (unexported, in the
// parent package): decode YAML to a generic map, re-encode as JSON, and
// validate against the module's JSON schema. It exists here so schema-shape
// tests can pin behavior directly against v0.New().Schema() without routing
// through the full translator.
func validateAgainstSchema(t *testing.T, schemaJSON, yamlData []byte) error {
	t.Helper()

	var yamlMap map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &yamlMap); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	jsonData, err := json.Marshal(yamlMap)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaJSON),
		gojsonschema.NewBytesLoader(jsonData),
	)
	if err != nil {
		t.Fatalf("gojsonschema.Validate() error = %v", err)
	}
	if !result.Valid() {
		msgs := make([]string, 0, len(result.Errors()))
		for _, e := range result.Errors() {
			msgs = append(msgs, e.String())
		}
		return fmt.Errorf("schema validation failed: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// TestMap_PreinstalledLifecycle_SchemaAcceptsKey: the Lifecycle schema
// definition must accept an optional "preinstalled" key, the same shape as
// every other lifecycle key (install/update/execute/stop/uninstall).
func TestMap_PreinstalledLifecycle_SchemaAcceptsKey(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: preinstalled-schema-test
targets:
  "*":
    lifecycle:
      preinstalled:
        - type: run
          command: "echo preinstall"
      install:
        - type: run
          command: "echo install"
`)
	if err := validateAgainstSchema(t, v0.New().Schema(), yamlData); err != nil {
		t.Fatalf("schema validation error = %v, want nil: preinstalled must be an accepted lifecycle key", err)
	}
}

// TestMap_PreinstalledLifecycle_PopulatesField: a preinstalled: block with a
// run-type step parses through Map() and lands in
// domain.TargetLifecycle.Preinstalled, exactly as install/update/etc. do.
func TestMap_PreinstalledLifecycle_PopulatesField(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: preinstalled-map-test
targets:
  "*":
    lifecycle:
      preinstalled:
        - type: run
          command: "echo preinstall"
          title: "Preinstall check"
          timeout: 10s
      install:
        - type: run
          command: "echo install"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	pre := precompiled["*"].Lifecycle.Preinstalled
	if len(pre) != 1 {
		t.Fatalf("Preinstalled steps = %d, want 1", len(pre))
	}
	runStep, ok := pre[0].(step.RunStep)
	if !ok {
		t.Fatalf("Preinstalled[0] is %T, want RunStep", pre[0])
	}
	if runStep.Command.Default != "echo preinstall" {
		t.Errorf("Command default = %q, want %q", runStep.Command.Default, "echo preinstall")
	}
}

// TestMap_PreinstalledLifecycle_AbsentYieldsEmptyStepList: manifests with no
// preinstalled key — i.e. every existing arrow fixture in this repo — must
// keep parsing exactly as before, with Preinstalled defaulting to empty, not
// an error. Pins zero migration impact on existing arrows.
func TestMap_PreinstalledLifecycle_AbsentYieldsEmptyStepList(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: no-preinstalled-test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "echo install"
      uninstall:
        - type: run
          command: "echo uninstall"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(precompiled["*"].Lifecycle.Preinstalled) != 0 {
		t.Errorf("Preinstalled = %v, want empty", precompiled["*"].Lifecycle.Preinstalled)
	}
}

func TestModule_Version(t *testing.T) {
	if v0.New().Version() != "v0" {
		t.Errorf("Version() = %q, want v0", v0.New().Version())
	}
}

// The schema still declares a version property purely so the key is tolerated —
// Metadata sets additionalProperties:false, so dropping it would turn a stray
// version into a hard validation error. Neither metadataV0 nor the aggregate has
// a matching field, so the authored value has nowhere to land: an arrow's
// version is the ref it resolves at. This pins that such a manifest parses clean.
func TestMap_MetadataVersion_IsToleratedAndIgnored(t *testing.T) {
	manifest := []byte(`schema: "arrow@v0"
metadata:
  name: legacy
  version: "9.9.9"
targets:
  "linux/amd64":
    lifecycle:
      install:
        - type: run
          command: "echo hi"
`)

	arrow, _, err := v0.Map(manifest)
	if err != nil {
		t.Fatalf("Map() error = %v, want nil: a manifest carrying metadata.version must parse, not be rejected", err)
	}
	if arrow.Name != "legacy" {
		t.Errorf("Name = %q, want legacy", arrow.Name)
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
	arrow, targets, err := v0.New().Parse(yamlData)
	if err == nil {
		t.Fatal("expected error for pre-refactor manifest shape")
	}
	if arrow != nil || targets != nil {
		t.Errorf("a rejected manifest must produce nothing, got arrow %v and targets %v", arrow, targets)
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
	control := []byte(strings.Replace(string(yamlData), "type: dependencies", "type: run\n          command: echo ok", 1))
	if _, _, err := v0.New().Parse(control); err != nil {
		t.Fatalf("the same manifest with a run step must parse, got %v", err)
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

func TestMap_FromToSteps(t *testing.T) {
	testCases := []struct {
		kind     string
		extra    string
		wantName string
	}{
		{kind: "extract"},
		{kind: "portable"},
		{kind: "portable", extra: "\n          name: tool.exe", wantName: "tool.exe"},
	}

	for _, tc := range testCases {
		t.Run(tc.kind+tc.wantName, func(t *testing.T) {
			yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: from-to-test
targets:
  "*":
    lifecycle:
      install:
        - type: ` + tc.kind + `
          title: Placing files
          timeout: 5m
          exit_on_failure: false
          from:
            default: "./default"
            linux/amd64: "./linux-amd64"
            darwin/arm64: "./darwin-arm64"
          to:
            default: "./"
            linux/amd64: "./bin/"` + tc.extra + `
`)
			if err := validateAgainstSchema(t, v0.New().Schema(), yamlData); err != nil {
				t.Fatalf("schema validation error = %v, want nil", err)
			}
			_, precompiled, err := v0.New().Parse(yamlData)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			steps := precompiled["*"].Lifecycle.Install
			if len(steps) != 1 {
				t.Fatalf("Install steps = %d, want 1", len(steps))
			}

			var from, to, timeout step.Overrideable[string]
			var name string
			switch got := steps[0].(type) {
			case step.ExtractStep:
				from, to, timeout = got.From, got.To, got.Timeout
			case step.PortableStep:
				from, to, timeout, name = got.From, got.To, got.Timeout, got.Name
			default:
				t.Fatalf("install[0] is %T, want %s step", steps[0], tc.kind)
			}
			if steps[0].Title() != "Placing files" || steps[0].ExitOnFailure() {
				t.Errorf("Title() = %q, ExitOnFailure() = %v", steps[0].Title(), steps[0].ExitOnFailure())
			}
			if timeout.Default != "5m" || name != tc.wantName {
				t.Errorf("Timeout default = %q, Name = %q", timeout.Default, name)
			}
			if from.Default != "./default" || from.OSArch["linux/amd64"] != "./linux-amd64" || from.OSArch["darwin/arm64"] != "./darwin-arm64" {
				t.Errorf("From = %+v", from)
			}
			if to.Default != "./" || to.OSArch["linux/amd64"] != "./bin/" {
				t.Errorf("To = %+v", to)
			}
		})
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

func TestMap_Expose_SchemaAcceptsKey(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: expose-schema-test
targets:
  "*":
    expose:
      cli:
        - name: mytool
          path: "${INSTALL_PATH}/bin/mytool"
      desktop:
        - name: MyApp
          path: auto
          icon: "${INSTALL_PATH}/icon.png"
          categories: [Utility]
    lifecycle:
      install:
        - type: run
          command: "echo install"
      uninstall:
        - type: run
          command: "echo uninstall"
`)
	if err := validateAgainstSchema(t, v0.New().Schema(), yamlData); err != nil {
		t.Fatalf("schema validation error = %v, want nil: expose must be an accepted target key", err)
	}
}

func TestMap_Expose_PopulatesField(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: expose-test
targets:
  "*":
    expose:
      cli:
        - name: mytool
          path: "${INSTALL_PATH}/bin/mytool"
      desktop:
        - name: MyApp
          path: auto
          icon: "${INSTALL_PATH}/icon.png"
          categories: [Utility, Development]
    lifecycle:
      install:
        - type: run
          command: "echo install"
      uninstall:
        - type: run
          command: "echo uninstall"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	expose := precompiled["*"].Expose
	if len(expose.CLI) != 1 || expose.CLI[0].Name != "mytool" || expose.CLI[0].Path != "${INSTALL_PATH}/bin/mytool" {
		t.Fatalf("Expose.CLI = %+v, want one mytool entry", expose.CLI)
	}
	if len(expose.Desktop) != 1 {
		t.Fatalf("Expose.Desktop = %+v, want one entry", expose.Desktop)
	}
	got := expose.Desktop[0]
	if got.Name != "MyApp" || got.Path != "auto" || got.Icon != "${INSTALL_PATH}/icon.png" {
		t.Fatalf("Expose.Desktop[0] = %+v, want MyApp/auto/icon", got)
	}
	if len(got.Categories) != 2 || got.Categories[0] != "Utility" || got.Categories[1] != "Development" {
		t.Fatalf("Expose.Desktop[0].Categories = %v, want [Utility Development]", got.Categories)
	}
}

func TestMap_Expose_AbsentYieldsZeroValue(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: expose-absent-test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "echo ok"
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !precompiled["*"].Expose.IsEmpty() {
		t.Fatalf("Expose = %+v, want empty", precompiled["*"].Expose)
	}
}

func TestMap_Expose_RejectsUnknownField(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: expose-unknown-field-test
targets:
  "*":
    expose:
      cli:
        - name: mytool
          path: "${INSTALL_PATH}/bin/mytool"
          unknown: nope
    lifecycle:
      install:
        - type: run
          command: "echo ok"
`)
	if err := validateAgainstSchema(t, v0.New().Schema(), yamlData); err == nil {
		t.Fatal("expected schema validation error for unknown expose entry field")
	}
}

func TestMap_Expose_ExplicitEmptyChildListReplacesParent(t *testing.T) {
	yamlData := []byte(`
schema: "arrow@v0"
metadata:
  name: expose-empty-child-test
targets:
  _base:
    expose:
      cli:
        - name: basetool
          path: "${INSTALL_PATH}/bin/basetool"
    lifecycle:
      install:
        - type: run
          command: "echo ok"
      uninstall:
        - type: run
          command: "echo bye"
  linux/amd64:
    base: _base
    expose:
      cli: []
    lifecycle: {}
`)
	_, precompiled, err := v0.New().Parse(yamlData)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	rt, err := v0.SelectTarget(precompiled, domain.OSLinuxAMD64)
	if err != nil {
		t.Fatalf("SelectTarget() error = %v", err)
	}
	if rt.Expose.CLI == nil || len(rt.Expose.CLI) != 0 {
		t.Fatalf("Expose.CLI = %v, want non-nil empty (explicit cli: [] must replace parent's basetool entry)", rt.Expose.CLI)
	}
}

func TestMap_Generator(t *testing.T) {
	testCases := []struct {
		name     string
		metadata string
		want     *domain.ArrowGenerator
	}{
		{
			name:     "absent generator is nil",
			metadata: "  name: plain\n",
			want:     nil,
		},
		{
			name:     "generator with warnings",
			metadata: "  name: forged\n  generator:\n    name: fletcher/1\n    confidence: medium\n    warnings: [assumed_arch, emulated]\n",
			want: &domain.ArrowGenerator{
				Name:       "fletcher/1",
				Confidence: "medium",
				Warnings:   []string{"assumed_arch", "emulated"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("schema: \"arrow@v0\"\nmetadata:\n" + tc.metadata +
				"targets:\n  \"*\":\n    lifecycle:\n      execute:\n        - type: run\n          command: \"echo hi\"\n")
			require.NoError(t, validateAgainstSchema(t, v0.New().Schema(), data))

			arrow, _, err := v0.New().Parse(data)

			require.NoError(t, err)
			assert.Equal(t, tc.want, arrow.Generator)
		})
	}
}

func TestMap_Generator_SchemaRejectsInvalidShapes(t *testing.T) {
	testCases := []struct {
		name      string
		generator string
	}{
		{name: "unknown confidence", generator: "    name: fletcher/1\n    confidence: certain\n"},
		{name: "missing name", generator: "    confidence: high\n"},
		{name: "non-string warning", generator: "    name: fletcher/1\n    confidence: low\n    warnings: [{a: b}]\n"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte("schema: \"arrow@v0\"\nmetadata:\n  name: forged\n  generator:\n" + tc.generator +
				"targets:\n  \"*\":\n    lifecycle:\n      execute:\n        - type: run\n          command: \"echo hi\"\n")

			assert.Error(t, validateAgainstSchema(t, v0.New().Schema(), data))
		})
	}
}

const uiManifestHead = `
schema: "arrow@v0"
metadata:
  name: ui-test
targets:
  "*":
    lifecycle:
      execute:
        - type: ui
`

func TestMap_UIStep_ListenCompilesToUIStep(t *testing.T) {
	data := []byte(uiManifestHead + `          title: Chat
          listen: [unix]
          path: /
`)
	require.NoError(t, validateAgainstSchema(t, v0.New().Schema(), data))

	_, precompiled, err := v0.New().Parse(data)
	require.NoError(t, err)

	execute := precompiled["*"].Lifecycle.Execute
	require.Len(t, execute, 1)
	got, ok := execute[0].(step.UIStep)
	require.True(t, ok, "execute[0] is %T, want UIStep", execute[0])
	require.Equal(t, step.NewUIStep("Chat", []string{"unix"}, "", "/", true), got)
}

func TestMap_UIStep_UpstreamRejectedBySchema(t *testing.T) {
	data := []byte(uiManifestHead + `          listen: [unix]
          upstream: x
`)
	require.Error(t, validateAgainstSchema(t, v0.New().Schema(), data))
}

func TestMap_UIStep_UnknownListenKindRejectedBySchema(t *testing.T) {
	data := []byte(uiManifestHead + `          listen: [tcp]
`)
	require.Error(t, validateAgainstSchema(t, v0.New().Schema(), data))
}

func TestMap_UIStep_StaticCompilesToUIStep(t *testing.T) {
	data := []byte(uiManifestHead + `          title: Docs
          static: ./dist
`)
	require.NoError(t, validateAgainstSchema(t, v0.New().Schema(), data))

	_, precompiled, err := v0.New().Parse(data)
	require.NoError(t, err)

	execute := precompiled["*"].Lifecycle.Execute
	require.Len(t, execute, 1)
	require.Equal(t, step.NewUIStep("Docs", nil, "./dist", "", true), execute[0])
}
