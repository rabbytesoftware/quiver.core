package rules

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestAllPrecompile_ReturnsAllRules(t *testing.T) {
	rules := AllPrecompile()
	if len(rules) == 0 {
		t.Fatal("expected AllPrecompile() to return at least one rule")
	}
	for _, r := range rules {
		if r.Name() == "" {
			t.Fatalf("precompile rule has empty name: %T", r)
		}
	}
}

func TestAllCompiled_ReturnsAllRules(t *testing.T) {
	rules := AllCompiled()
	if len(rules) == 0 {
		t.Fatal("expected AllCompiled() to return at least one rule")
	}
	for _, r := range rules {
		if r.Name() == "" {
			t.Fatalf("compiled rule has empty name: %T", r)
		}
	}
}

func TestPrecompileRuleNames(t *testing.T) {
	cases := []struct {
		rule PrecompileRule
		want string
	}{
		{VariablesRule{}, "variables"},
		{NetbridgeRule{}, "netbridge"},
		{BaseIntegrityRule{}, "base_integrity"},
		{OverrideableKeysRule{}, "overrideable_keys"},
		{OverrideableCoverageRule{}, "overrideable_coverage"},
	}
	for _, tc := range cases {
		if got := tc.rule.Name(); got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.rule, got, tc.want)
		}
	}
}

func TestCompiledRuleNames(t *testing.T) {
	cases := []struct {
		rule CompiledRule
		want string
	}{
		{ToolsServicesRule{}, "tools_services"},
		{ExportStaticRule{}, "export_static"},
		{VariableRefsRule{}, "variable_refs"},
		{ServicePackageRule{}, "service_package"},
		{LifecyclePairsRule{}, "lifecycle_pairs"},
		{TimeoutFormatRule{}, "timeout_format"},
		{MethodStatesRule{}, "method_states"},
	}
	for _, tc := range cases {
		if got := tc.rule.Name(); got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.rule, got, tc.want)
		}
	}
}

func TestRunPrecompile_ValidManifest_ReturnsNil(t *testing.T) {
	m := &domain.ArrowManifest{}
	err := RunPrecompile(context.Background(), m, map[string]models.PrecompiledTarget{})
	if err != nil {
		t.Fatalf("expected nil for valid manifest, got: %v", err)
	}
}

func TestRunCompiled_ValidManifest_ReturnsNil(t *testing.T) {
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "echo ok", false, "10s", true),
					},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	err := RunCompiled(context.Background(), m)
	if err != nil {
		t.Fatalf("expected nil for valid manifest, got: %v", err)
	}
}

func TestRunPrecompile_InvalidManifest_ReturnsError(t *testing.T) {
	m := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: ""},
		},
	}
	err := RunPrecompile(context.Background(), m, map[string]models.PrecompiledTarget{})
	if err == nil {
		t.Fatal("expected error for invalid manifest, got nil")
	}
}

func TestRunCompiled_InvalidManifest_ReturnsError(t *testing.T) {
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Exports: map[string]string{
					"PATH": "${SOME_VAR}/bin",
				},
			},
		},
	}
	err := RunCompiled(context.Background(), m)
	if err == nil {
		t.Fatal("expected error for manifest with variable ref in export, got nil")
	}
}
