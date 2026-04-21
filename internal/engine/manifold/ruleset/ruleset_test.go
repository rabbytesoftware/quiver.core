package ruleset_test

import (
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow/v0"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func mustCompile(t *testing.T, manifest *domain.Arrow, precompiled map[string]models.PrecompiledTarget) {
	t.Helper()
	if err := compiler.New().Compile(manifest, precompiled, v0.New().Selector()); err != nil {
		t.Fatalf("compiler.Compile() unexpected error: %v", err)
	}
}

func compileIgnoringError(t *testing.T, manifest *domain.Arrow, precompiled map[string]models.PrecompiledTarget) {
	t.Helper()
	compiler.New().Compile(manifest, precompiled, v0.New().Selector()) //nolint:errcheck
}

func makePrecompiledInstall() models.PrecompiledTarget {
	return models.PrecompiledTarget{
		Lifecycle: domain.TargetLifecycle{
			Install: step.StepList{
				step.NewRunStep("install", "echo ok", false, "10s", true),
			},
			Uninstall: step.StepList{},
		},
	}
}

func makePrecompiledService() models.PrecompiledTarget {
	return models.PrecompiledTarget{
		Lifecycle: domain.TargetLifecycle{
			Install: step.StepList{
				step.NewRunStep("install", "echo ok", false, "10s", true),
			},
			Execute: step.StepList{
				step.NewRunStep("run", "./server", false, "10s", true),
			},
			Stop: step.StepList{
				step.NewSignalStep("stop", "graceful", "10s", true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("uninstall", "echo bye", false, "10s", true),
			},
		},
	}
}

func assertRuleError(t *testing.T, err error, rule string) {
	t.Helper()
	if !errors.Is(err, ruleset.ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got: %v", err)
	}
	var asmErrs ruleset.RuleErrors
	if !errors.As(err, &asmErrs) {
		t.Fatalf("expected RuleErrors, got: %T", err)
	}
	for _, ae := range asmErrs {
		if ae.Rule == rule {
			return
		}
	}
	t.Fatalf("expected rule %q in errors, got: %v", rule, asmErrs)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestRule_InstallWithoutUninstall(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "missing_pair")
}

func TestRule_InstallWithoutUninstallIsInvalid(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err == nil {
		t.Fatal("install with no uninstall steps should fail lifecycle_pairs rule")
	}
}

func TestRule_BothEmptyLifecycleIsValid(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("both empty lifecycle should be valid: %v", err)
	}
}

func TestRule_ExecuteWithoutStop_IsValid(t *testing.T) {
	// Tools run and exit on their own — stop is not required.
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", false)},
				Execute:   step.StepList{step.NewRunStep("run", "echo done", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("execute without stop should be valid for tools: %v", err)
	}
}

func TestRule_StopWithoutExecute_IsInvalid(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Stop: step.StepList{step.NewRunStep("stop", "echo stopped", false, "10s", false)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "missing_pair")
}

func TestRule_MixedKind(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"linux/*":  makePrecompiledService(),
		"darwin/*": makePrecompiledInstall(),
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "mixed_kind")
}

func TestRule_NoSupportedPlatform(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"_abstract": makePrecompiledInstall(),
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	compileIgnoringError(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "no_supported_platform")
}

func TestRule_InvalidTimeout(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "2h", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "invalid_timeout")
}

func TestRule_InvalidState(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: makePrecompiledInstall().Lifecycle,
			Methods: map[string]domain.Method{
				"restart": {
					AvailableIn: []domain.ArrowState{"stopped"},
					Steps:       step.StepList{},
				},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "invalid_state")
}

func TestRule_ToolsServicesOverlap(t *testing.T) {
	ns := domain.Namespace("github.com/user/repo/tool")
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Tools:     []domain.Namespace{ns},
			Services:  []domain.Namespace{ns},
			Lifecycle: makePrecompiledInstall().Lifecycle,
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "tools_services_overlap")
}

func TestRule_ExportVarInterp(t *testing.T) {
	// After compilation, exports are resolved strings.
	// ExportStaticRule checks compiled export values.
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Exports:   map[string]step.Overrideable[string]{"bin": {Default: "${INSTALL_PATH}/bin"}},
			Lifecycle: makePrecompiledInstall().Lifecycle,
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "export_var_interpolation")
}

func TestRule_UnresolvedVar(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "${UNKNOWN_VAR}/bin/install", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "unresolved_variable")
}

func TestValidateArrow_ValidManifest(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "${MY_VAR}/bin/install", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{
		Variables: []domain.Variable{{Name: "MY_VAR"}},
	}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArrow_ExactTargetKey(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"linux/amd64": makePrecompiledService(),
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("exact target key: unexpected error: %v", err)
	}
}

func TestValidateQuiver_ReturnsNil(t *testing.T) {
	err := ruleset.ValidateQuiver(&domain.QuiverManifest{})
	if err != nil {
		t.Fatalf("ValidateQuiver() unexpected error: %v", err)
	}
}

func TestRule_BaseCycle_CompilerCatchesIt(t *testing.T) {
	// Base cycles are caught by the compiler, not by validation rules.
	precompiled := map[string]models.PrecompiledTarget{
		"_a":      {Base: "_b"},
		"_b":      {Base: "_a"},
		"linux/*": {Base: "_a", Lifecycle: makePrecompiledInstall().Lifecycle},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	err := compiler.New().Compile(manifest, precompiled, v0.New().Selector())
	if err == nil {
		t.Fatal("expected compiler error for base cycle")
	}
}

func TestRule_FetchStepUnresolvedVar(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewFetchStep("dl", "https://example.com/${UNKNOWN_VAR}/file", "./file", "", "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "unresolved_variable")
}

func TestRule_MethodFetchStepUnresolvedVar(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: makePrecompiledInstall().Lifecycle,
			Methods: map[string]domain.Method{
				"update": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
					Steps: step.StepList{
						step.NewFetchStep("dl", "https://example.com/${UNKNOWN_UPDATE_VAR}/file", "./file", "", "10s", true),
					},
				},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "unresolved_variable")
}

func TestRule_NetbridgeVarKnown(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Execute:   step.StepList{step.NewRunStep("run", "./server --port ${GAME_PORT}", false, "10s", true)},
				Stop:      step.StepList{step.NewSignalStep("stop", "graceful", "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{
		Netbridge: []netbridge.PortDef{
			{Name: "GAME_PORT", Protocol: "tcp", Default: 27015, Required: true},
		},
	}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("netbridge var should be known: unexpected error: %v", err)
	}
}

func TestRule_DottedTokenSkipped(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "${env.MY_PATH}/bin/install", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("dotted token should be skipped: unexpected error: %v", err)
	}
}

func TestRule_TimeoutInMethod(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: makePrecompiledInstall().Lifecycle,
			Methods: map[string]domain.Method{
				"check": {
					AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
					Steps:       step.StepList{step.NewRunStep("check", "check.sh", false, "2h", true)},
				},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "invalid_timeout")
}

func TestRule_FetchStepTimeout(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewFetchStep("dl", "https://example.com/file", "./file", "", "2h", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "invalid_timeout")
}

func TestRule_SignalStepTimeout(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Execute:   step.StepList{step.NewRunStep("run", "./server", false, "10s", true)},
				Stop:      step.StepList{step.NewSignalStep("stop", "graceful", "2h", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	assertRuleError(t, err, "invalid_timeout")
}

func TestRule_DependenciesStep_NoTimeout(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewDependenciesStep("install deps")},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", false)},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	mustCompile(t, manifest, precompiled)
	rls := ruleset.New()
	err := rls.ValidateCompiled(manifest)
	if err != nil {
		t.Fatalf("DependenciesStep should not produce a timeout error: %v", err)
	}
}

func TestRuleError_Error_FormatsCorrectly(t *testing.T) {
	ae := ruleset.RuleError{
		Field:   "targets[*].lifecycle.install",
		Rule:    "missing_pair",
		Message: "install and uninstall must both be defined or both be empty",
	}
	got := ae.Error()
	if got == "" {
		t.Fatal("expected non-empty error string")
	}
	if !errors.Is(ae, ruleset.ErrInvalidManifest) {
		t.Fatal("expected RuleError to unwrap to ErrInvalidManifest")
	}
}

func TestValidatePrecompile_Valid(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("uninstall", "echo bye", false, "10s", false),
				},
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	rls := ruleset.New()
	err := rls.ValidatePrecompile(manifest, precompiled)
	if err != nil {
		t.Fatalf("expected no errors for valid precompiled targets, got: %v", err)
	}
}

func TestValidatePrecompile_InvalidKeys_ReturnsError(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Exports: map[string]step.Overrideable[string]{
				"BIN": {OSArch: map[string]string{"linux": "/usr/bin/foo"}}, // bare key, invalid
			},
		},
	}
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	rls := ruleset.New()
	err := rls.ValidatePrecompile(manifest, precompiled)
	if err == nil {
		t.Fatal("expected error for invalid precompile key, got nil")
	}
}

func TestValidatePrecompile_EmptyPrecompiled(t *testing.T) {
	rls := ruleset.New()
	m := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test.arrow"}}
	err := rls.ValidatePrecompile(m, map[string]models.PrecompiledTarget{})
	if err != nil {
		t.Fatalf("expected no errors for empty precompiled, got: %v", err)
	}
}

func TestRuleErrors_Error_JoinsAllMessages(t *testing.T) {
	errs := ruleset.RuleErrors{
		{Field: "targets[*].lifecycle.install", Rule: "missing_pair", Message: "msg1"},
		{Field: "targets[*].lifecycle.execute", Rule: "missing_pair", Message: "msg2"},
	}
	got := errs.Error()
	if got == "" {
		t.Fatal("expected non-empty joined error string")
	}
}
