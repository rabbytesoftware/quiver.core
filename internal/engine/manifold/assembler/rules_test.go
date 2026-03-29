package assembler

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

// ─── Rule 1: OS Compatibility ───────────────────────────────────────────────

func TestCheckOSCompatibility(t *testing.T) {
	supported := []string{"linux/amd64", "darwin/amd64"}

	tests := []struct {
		name    string
		os      string
		wantErr error
	}{
		{"supported linux", "linux/amd64", nil},
		{"supported darwin", "darwin/amd64", nil},
		{"unsupported windows", "windows/amd64", ErrUnsupportedPlatform},
		{"empty os", "", ErrUnsupportedPlatform},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkOSCompatibility(tc.os, supported)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("checkOSCompatibility() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckOSCompatibility_EmptySupported(t *testing.T) {
	err := checkOSCompatibility("linux/amd64", []string{})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

// ─── Rule 2: OS Override Resolution ─────────────────────────────────────────

func boolPtr(b bool) *bool { return &b }

func TestResolveOverrides_NoOverrides(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{Type: "run", Command: "./install.sh"},
	}
	result, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Command != "./install.sh" {
		t.Errorf("Command = %q, want ./install.sh", result[0].Command)
	}
}

func TestResolveOverrides_AppliesMatchingOverride(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{
			Type:    "run",
			Command: "./install.sh",
			Overrides: map[string]models.RawStepOverride{
				"windows/amd64": {Command: `.\install.bat`, Title: "Win install"},
			},
		},
	}
	result, err := a.resolveOverrides(steps, "windows/amd64", []string{"linux/amd64", "windows/amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Command != `.\install.bat` {
		t.Errorf("Command = %q, want .\\install.bat", result[0].Command)
	}
	if result[0].Title != "Win install" {
		t.Errorf("Title = %q, want Win install", result[0].Title)
	}
	if result[0].Overrides != nil {
		t.Error("expected Overrides to be nil after resolution")
	}
}

func TestResolveOverrides_SkipsNonMatchingOverride(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{
			Type:    "run",
			Command: "./install.sh",
			Overrides: map[string]models.RawStepOverride{
				"windows/amd64": {Command: ".\\install.bat"},
			},
		},
	}
	result, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64", "windows/amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Command != "./install.sh" {
		t.Errorf("Command = %q, want ./install.sh", result[0].Command)
	}
	if result[0].Overrides != nil {
		t.Error("expected Overrides to be nil after resolution")
	}
}

func TestResolveOverrides_ErrorOnNonRunStep(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{
			Type: "fetch",
			URL:  "https://example.com",
			To:   "./file",
			Overrides: map[string]models.RawStepOverride{
				"linux/amd64": {Title: "override"},
			},
		},
	}
	_, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64"})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestResolveOverrides_ErrorOnUnknownOSKey(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{
			Type:    "run",
			Command: "./install.sh",
			Overrides: map[string]models.RawStepOverride{
				"freebsd/amd64": {Command: "./bsd.sh"},
			},
		},
	}
	_, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64"})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

func TestResolveOverrides_PartialOverride(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{
			Type:    "run",
			Command: "./install.sh",
			Title:   "Original title",
			Overrides: map[string]models.RawStepOverride{
				"linux/amd64": {Title: "Linux title"},
			},
		},
	}
	result, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].Command != "./install.sh" {
		t.Errorf("Command = %q, want ./install.sh (unchanged)", result[0].Command)
	}
	if result[0].Title != "Linux title" {
		t.Errorf("Title = %q, want Linux title", result[0].Title)
	}
}

func TestResolveOverrides_ExitOnFailureOverride(t *testing.T) {
	a := New()
	f := false
	steps := []models.RawStep{
		{
			Type:          "run",
			Command:       "./install.sh",
			ExitOnFailure: boolPtr(true),
			Overrides: map[string]models.RawStepOverride{
				"linux/amd64": {ExitOnFailure: &f},
			},
		},
	}
	result, err := a.resolveOverrides(steps, "linux/amd64", []string{"linux/amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0].ExitOnFailure == nil || *result[0].ExitOnFailure != false {
		t.Errorf("ExitOnFailure = %v, want false", result[0].ExitOnFailure)
	}
}

// ─── Rule 3: Lifecycle Pair Validation ──────────────────────────────────────

func TestValidateLifecyclePairs_BothDefined(t *testing.T) {
	lc := models.RawLifecycle{
		Install:   []models.RawStep{{Type: "run", Command: "x"}},
		Uninstall: []models.RawStep{{Type: "run", Command: "x"}},
		Execute:   []models.RawStep{{Type: "run", Command: "x"}},
		Stop:      []models.RawStep{{Type: "signal", Signal: "SIGTERM"}},
	}
	if err := validateLifecyclePairs(lc); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateLifecyclePairs_NoneDefinedIsValid(t *testing.T) {
	lc := models.RawLifecycle{}
	if err := validateLifecyclePairs(lc); err != nil {
		t.Errorf("unexpected error for empty lifecycle: %v", err)
	}
}

func TestValidateLifecyclePairs_InstallWithoutUninstall(t *testing.T) {
	lc := models.RawLifecycle{
		Install: []models.RawStep{{Type: "run", Command: "x"}},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for install without uninstall")
	}
}

func TestValidateLifecyclePairs_UninstallWithoutInstall(t *testing.T) {
	lc := models.RawLifecycle{
		Uninstall: []models.RawStep{{Type: "run", Command: "x"}},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for uninstall without install")
	}
}

func TestValidateLifecyclePairs_ExecuteWithoutStop(t *testing.T) {
	lc := models.RawLifecycle{
		Execute: []models.RawStep{{Type: "run", Command: "x"}},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for execute without stop")
	}
}

func TestValidateLifecyclePairs_StopWithoutExecute(t *testing.T) {
	lc := models.RawLifecycle{
		Stop: []models.RawStep{{Type: "signal", Signal: "SIGTERM"}},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for stop without execute")
	}
}

func TestValidateLifecyclePairs_InstallOnlyIsInvalid(t *testing.T) {
	lc := models.RawLifecycle{
		Install: []models.RawStep{{Type: "run", Command: "x"}},
		Execute: []models.RawStep{{Type: "run", Command: "x"}},
		Stop:    []models.RawStep{{Type: "signal", Signal: "SIGTERM"}},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for install without uninstall")
	}
}

// ─── Rule 4: Step Type Validation ───────────────────────────────────────────

func TestValidateStep_Run(t *testing.T) {
	a := New()
	tests := []struct {
		name    string
		step    models.RawStep
		wantErr bool
	}{
		{"run with command", models.RawStep{Type: "run", Command: "echo hi"}, false},
		{"run without command", models.RawStep{Type: "run"}, true},
		{"fetch with url and to", models.RawStep{Type: "fetch", URL: "https://x.com", To: "./f"}, false},
		{"fetch missing url", models.RawStep{Type: "fetch", To: "./f"}, true},
		{"fetch missing to", models.RawStep{Type: "fetch", URL: "https://x.com"}, true},
		{"signal with signal", models.RawStep{Type: "signal", Signal: "SIGTERM"}, false},
		{"signal missing signal", models.RawStep{Type: "signal"}, true},
		{"dependencies", models.RawStep{Type: "dependencies"}, false},
		{"unknown type", models.RawStep{Type: "copy"}, true},
		{"empty type", models.RawStep{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := a.validateStep(tc.step)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateStep() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil && !errors.Is(err, ErrInvalidManifest) {
				t.Errorf("expected ErrInvalidManifest, got %v", err)
			}
		})
	}
}

// ─── Rule 5: Dependency Validation ──────────────────────────────────────────

func TestValidateDependencies_Valid(t *testing.T) {
	deps := []string{
		"github.com/user/repo",
		"github.com/user/repo/arrow",
	}
	namespaces, err := validateDependencies(deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(namespaces) != 2 {
		t.Errorf("len = %d, want 2", len(namespaces))
	}
}

func TestValidateDependencies_Empty(t *testing.T) {
	namespaces, err := validateDependencies(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(namespaces) != 0 {
		t.Errorf("expected empty slice, got %v", namespaces)
	}
}

func TestValidateDependencies_Invalid(t *testing.T) {
	_, err := validateDependencies([]string{"not-a-valid-namespace"})
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
	}
}

// ─── Rule 6: Variable and Netbridge Validation ───────────────────────────────

func TestValidateVariables_Unique(t *testing.T) {
	vars := []domain.Variable{
		{Name: "A"},
		{Name: "B"},
	}
	if err := validateVariables(vars); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateVariables_Duplicate(t *testing.T) {
	vars := []domain.Variable{
		{Name: "A"},
		{Name: "A"},
	}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for duplicate variable names")
	}
}

func TestValidateVariables_SelectRequiresValues(t *testing.T) {
	vars := []domain.Variable{
		{Name: "CHOICE", Type: domain.VariableTypeSelect},
	}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for select without values")
	}
}

func TestValidateVariables_SelectWithValues(t *testing.T) {
	vars := []domain.Variable{
		{Name: "CHOICE", Type: domain.VariableTypeSelect, Values: []string{"a", "b"}},
	}
	if err := validateVariables(vars); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateNetbridge_Unique(t *testing.T) {
	ports := []netbridge.PortDef{
		{Name: "WEB", Protocol: netbridge.ProtocolTCP},
		{Name: "GAME", Protocol: netbridge.ProtocolUDP},
	}
	if err := validateNetbridge(ports); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateNetbridge_Duplicate(t *testing.T) {
	ports := []netbridge.PortDef{
		{Name: "WEB", Protocol: netbridge.ProtocolTCP},
		{Name: "WEB", Protocol: netbridge.ProtocolUDP},
	}
	if !errors.Is(validateNetbridge(ports), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for duplicate port names")
	}
}

func TestValidateVariables_EmptyName(t *testing.T) {
	vars := []domain.Variable{{Name: ""}}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for empty variable name")
	}
}

func TestValidateVariables_MinGreaterThanMax(t *testing.T) {
	vars := []domain.Variable{{Name: "X", Min: 10, Max: 5}}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for min > max")
	}
}

func TestValidateNetbridge_EmptyName(t *testing.T) {
	ports := []netbridge.PortDef{{Name: "", Protocol: netbridge.ProtocolTCP}}
	if !errors.Is(validateNetbridge(ports), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for empty port name")
	}
}

func TestValidateNetbridge_PortOutOfRange(t *testing.T) {
	ports := []netbridge.PortDef{{Name: "WEB", Protocol: netbridge.ProtocolTCP, Default: 70000}}
	if !errors.Is(validateNetbridge(ports), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for port out of range")
	}
}

func TestValidateNetbridge_InvalidProtocol(t *testing.T) {
	ports := []netbridge.PortDef{{Name: "WEB", Protocol: netbridge.Protocol("invalid")}}
	if !errors.Is(validateNetbridge(ports), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for invalid protocol")
	}
}

// ─── Rule 7: Timeout Parsing (tested via buildStep) ─────────────────────────

func TestValidateSteps_InvalidTimeout(t *testing.T) {
	a := New()
	steps := []models.RawStep{
		{Type: "run", Command: "echo", Timeout: "not-a-duration"},
	}
	_, err := a.buildSteps(steps)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest for invalid timeout, got %v", err)
	}
}

// ─── Rule 8: Method State Validation ────────────────────────────────────────

func TestValidateMethodStates_Valid(t *testing.T) {
	methods := map[string]models.RawMethod{
		"update": {AvailableIn: []string{"ready", "running"}},
	}
	if err := validateMethodStates(methods); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateMethodStates_InvalidState(t *testing.T) {
	methods := map[string]models.RawMethod{
		"update": {AvailableIn: []string{"stopped"}},
	}
	if !errors.Is(validateMethodStates(methods), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for invalid state")
	}
}

func TestValidateMethodStates_EmptyIsValid(t *testing.T) {
	if err := validateMethodStates(nil); err != nil {
		t.Errorf("unexpected error for nil methods: %v", err)
	}
}

func TestValidateMethodStates_NoAvailableIn(t *testing.T) {
	methods := map[string]models.RawMethod{
		"update": {AvailableIn: []string{}},
	}
	if err := validateMethodStates(methods); err != nil {
		t.Errorf("unexpected error for empty available_in: %v", err)
	}
}
