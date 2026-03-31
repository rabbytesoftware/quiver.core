package assembler

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

// ─── Rule 1: Lifecycle Pair Validation ──────────────────────────────────────

func TestValidateLifecyclePairs_BothPairsDefined(t *testing.T) {
	lc := domain.Lifecycle{
		Install: step.StepList{
			step.NewRunStep("Install", "x", 0, true),
		},
		Uninstall: step.StepList{
			step.NewRunStep("Uninstall", "x", 0, true),
		},
		Execute: step.StepList{
			step.NewRunStep("Execute", "x", 0, true),
		},
		Stop: step.StepList{
			step.NewSignalStep("Stop", "SIGTERM", 0, true),
		},
	}
	if err := validateLifecyclePairs(lc); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateLifecyclePairs_NoneDefinedIsValid(t *testing.T) {
	lc := domain.Lifecycle{}
	if err := validateLifecyclePairs(lc); err != nil {
		t.Errorf("unexpected error for empty lifecycle: %v", err)
	}
}

func TestValidateLifecyclePairs_InstallWithoutUninstall(t *testing.T) {
	lc := domain.Lifecycle{
		Install: step.StepList{
			step.NewRunStep("Install", "x", 0, true),
		},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for install without uninstall")
	}
}

func TestValidateLifecyclePairs_UninstallWithoutInstall(t *testing.T) {
	lc := domain.Lifecycle{
		Uninstall: step.StepList{
			step.NewRunStep("Uninstall", "x", 0, true),
		},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for uninstall without install")
	}
}

func TestValidateLifecyclePairs_ExecuteWithoutStop(t *testing.T) {
	lc := domain.Lifecycle{
		Execute: step.StepList{
			step.NewRunStep("Execute", "x", 0, true),
		},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for execute without stop")
	}
}

func TestValidateLifecyclePairs_StopWithoutExecute(t *testing.T) {
	lc := domain.Lifecycle{
		Stop: step.StepList{
			step.NewSignalStep("Stop", "SIGTERM", 0, true),
		},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for stop without execute")
	}
}

func TestValidateLifecyclePairs_InstallOnlyIsInvalid(t *testing.T) {
	lc := domain.Lifecycle{
		Install: step.StepList{
			step.NewRunStep("Install", "x", 0, true),
		},
		Execute: step.StepList{
			step.NewRunStep("Execute", "x", 0, true),
		},
		Stop: step.StepList{
			step.NewSignalStep("Stop", "SIGTERM", 0, true),
		},
	}
	if !errors.Is(validateLifecyclePairs(lc), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for install without uninstall")
	}
}

// ─── Rule 2: Dependency Validation ──────────────────────────────────────────

func TestValidateDependencies_Valid(t *testing.T) {
	deps := []domain.Namespace{
		domain.Namespace("github.com/user/repo"),
		domain.Namespace("github.com/user/repo/arrow"),
	}
	if err := validateDependencies(deps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDependencies_Empty(t *testing.T) {
	if err := validateDependencies(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDependencies_Invalid(t *testing.T) {
	deps := []domain.Namespace{
		domain.Namespace("not-a-valid-namespace"),
	}
	if !errors.Is(validateDependencies(deps), ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", validateDependencies(deps))
	}
}

// ─── Rule 3: Variable and Netbridge Validation ───────────────────────────────

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

// ─── Rule 4: Method State Validation ────────────────────────────────────────

func TestValidateMethodStates_Valid(t *testing.T) {
	methods := map[string]domain.Method{
		"update": {AvailableIn: []domain.ArrowState{domain.ArrowStateReady, domain.ArrowStateRunning}},
	}
	if err := validateMethodStates(methods); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateMethodStates_InvalidState(t *testing.T) {
	methods := map[string]domain.Method{
		"update": {AvailableIn: []domain.ArrowState{domain.ArrowState("stopped")}},
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
	methods := map[string]domain.Method{
		"update": {AvailableIn: []domain.ArrowState{}},
	}
	if err := validateMethodStates(methods); err != nil {
		t.Errorf("unexpected error for empty available_in: %v", err)
	}
}
