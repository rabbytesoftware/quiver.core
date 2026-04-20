package ruleset

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/rules"
)

// helpers to call rule structs with a simple manifest wrapper

func validateVariables(vars []domain.Variable) error {
	errs := rules.VariablesRule{}.Validate(&domain.Arrow{Variables: vars}, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateNetbridge(ports []netbridge.PortDef) error {
	errs := rules.NetbridgeRule{}.Validate(&domain.Arrow{Netbridge: ports}, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validateServicePackageConsistency(targets map[domain.OS]domain.Target) RuleErrors {
	return rules.ServicePackageRule{}.Validate(&domain.Arrow{Targets: targets})
}

func validateLifecyclePairs(target domain.Target, key string) RuleErrors {
	return rules.LifecyclePairsRule{}.Validate(&domain.Arrow{
		Targets: map[domain.OS]domain.Target{domain.OS(key): target},
	})
}

func validateMethodStates(target domain.Target, key string) RuleErrors {
	return rules.MethodStatesRule{}.Validate(&domain.Arrow{
		Targets: map[domain.OS]domain.Target{domain.OS(key): target},
	})
}

// ─── validateVariables ───────────────────────────────────────────────────────

func TestValidateVariables_Unique(t *testing.T) {
	vars := []domain.Variable{{Name: "A"}, {Name: "B"}}
	if err := validateVariables(vars); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateVariables_Duplicate(t *testing.T) {
	vars := []domain.Variable{{Name: "A"}, {Name: "A"}}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for duplicate variable names")
	}
}

func TestValidateVariables_SelectRequiresValues(t *testing.T) {
	vars := []domain.Variable{{Name: "CHOICE", Type: domain.VariableTypeSelect}}
	if !errors.Is(validateVariables(vars), ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for select without values")
	}
}

func TestValidateVariables_SelectWithValues(t *testing.T) {
	vars := []domain.Variable{{Name: "CHOICE", Type: domain.VariableTypeSelect, Values: []string{"a", "b"}}}
	if err := validateVariables(vars); err != nil {
		t.Errorf("unexpected error: %v", err)
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

// ─── validateNetbridge ───────────────────────────────────────────────────────

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

// ─── validateLifecyclePairs (compiled) ───────────────────────────────────────

func TestValidateLifecyclePairs_BothPairsDefined(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install:   step.StepList{step.NewRunStep("i", "x", false, "10s", true)},
			Uninstall: step.StepList{step.NewRunStep("u", "x", false, "10s", true)},
			Execute:   step.StepList{step.NewRunStep("e", "x", false, "10s", true)},
			Stop:      step.StepList{step.NewSignalStep("s", "SIGTERM", "10s", true)},
		},
	}
	if errs := validateLifecyclePairs(target, "*"); len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateLifecyclePairs_NoneDefinedIsValid(t *testing.T) {
	target := domain.Target{}
	if errs := validateLifecyclePairs(target, "*"); len(errs) != 0 {
		t.Errorf("unexpected errors for empty lifecycle: %v", errs)
	}
}

func TestValidateLifecyclePairs_InstallWithoutUninstall(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install: step.StepList{step.NewRunStep("i", "x", false, "10s", true)},
		},
	}
	errs := validateLifecyclePairs(target, "*")
	if !errors.Is(errs, ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for install without uninstall")
	}
}

func TestValidateLifecyclePairs_ExecuteWithoutStop_IsValid(t *testing.T) {
	// Tools can have execute without stop — they run and exit on their own.
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install:   step.StepList{step.NewRunStep("i", "x", false, "10s", true)},
			Uninstall: step.StepList{step.NewRunStep("u", "x", false, "10s", true)},
			Execute:   step.StepList{step.NewRunStep("e", "x", false, "10s", true)},
		},
	}
	errs := validateLifecyclePairs(target, "*")
	if errors.Is(errs, ErrInvalidManifest) {
		t.Error("execute without stop should be valid for tools; got unexpected error")
	}
}

func TestValidateLifecyclePairs_StopWithoutExecute_IsInvalid(t *testing.T) {
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Stop: step.StepList{step.NewRunStep("s", "x", false, "10s", false)},
		},
	}
	errs := validateLifecyclePairs(target, "*")
	if !errors.Is(errs, ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for stop without execute")
	}
}

// ─── validateMethodStates ────────────────────────────────────────────────────

func TestValidateMethodStates_Valid(t *testing.T) {
	target := domain.Target{
		Methods: map[string]domain.Method{
			"update": {AvailableIn: []domain.ArrowState{domain.ArrowStateReady, domain.ArrowStateRunning}},
		},
	}
	if errs := validateMethodStates(target, "*"); len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateMethodStates_InvalidState(t *testing.T) {
	target := domain.Target{
		Methods: map[string]domain.Method{
			"update": {AvailableIn: []domain.ArrowState{domain.ArrowState("stopped")}},
		},
	}
	errs := validateMethodStates(target, "*")
	if !errors.Is(errs, ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for invalid state")
	}
}

func TestValidateMethodStates_EmptyIsValid(t *testing.T) {
	target := domain.Target{}
	if errs := validateMethodStates(target, "*"); len(errs) != 0 {
		t.Errorf("unexpected errors for nil methods: %v", errs)
	}
}

func TestValidateMethodStates_NoAvailableIn(t *testing.T) {
	target := domain.Target{
		Methods: map[string]domain.Method{
			"update": {AvailableIn: []domain.ArrowState{}},
		},
	}
	if errs := validateMethodStates(target, "*"); len(errs) != 0 {
		t.Errorf("unexpected errors for empty available_in: %v", errs)
	}
}

// ─── validateServicePackageConsistency ───────────────────────────────────────

func TestValidateServicePackageConsistency_AllService(t *testing.T) {
	targets := map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {
			Lifecycle: domain.TargetLifecycle{
				Execute: step.StepList{step.NewRunStep("e", "x", false, "10s", true)},
			},
		},
	}
	errs := validateServicePackageConsistency(targets)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestValidateServicePackageConsistency_Mixed(t *testing.T) {
	targets := map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {
			Lifecycle: domain.TargetLifecycle{
				Execute: step.StepList{step.NewRunStep("e", "x", false, "10s", true)},
			},
		},
		domain.OSDarwinAMD64: {},
	}
	errs := validateServicePackageConsistency(targets)
	if !errors.Is(errs, ErrInvalidManifest) {
		t.Error("expected ErrInvalidManifest for mixed service/package")
	}
}
