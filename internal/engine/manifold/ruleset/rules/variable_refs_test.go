package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestVariableRefsRule_Valid_KnownVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: "MY_VAR"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${MY_VAR}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for known variable ref, got: %v", errs)
	}
}

func TestVariableRefsRule_Valid_BuiltinVars(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${INSTALL_PATH}/bin && ${ARROW_NAMESPACE} && ${PLATFORM}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for builtin vars, got: %v", errs)
	}
}

func TestVariableRefsRule_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${UNKNOWN_VAR}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved variable, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_NetbridgeVarKnown(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "GAME_PORT", Protocol: "tcp", Default: 27015},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "./server --port ${GAME_PORT}", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("netbridge var should be known, got: %v", errs)
	}
}

func TestVariableRefsRule_DottedTokenSkipped(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "${env.MY_PATH}/bin/install", false, "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("dotted token should be skipped, got: %v", errs)
	}
}

func TestVariableRefsRule_FetchStep_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("dl", "https://example.com/${UNKNOWN_VAR}/file", "./file", "", "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in fetch step URL, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_FetchStep_ToUnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewFetchStep("dl", "https://example.com/file", "${UNKNOWN_DEST}/file", "", "10s", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in fetch step to field, got none")
	}
}

func TestVariableRefsRule_MethodStep_UnresolvedVar(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Methods: map[string]domain.Method{
					"update": {
						AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
						Steps: step.StepList{
							step.NewRunStep("update", "${UNKNOWN_UPDATE_VAR}/update.sh", false, "10s", true),
						},
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for unresolved var in method step, got none")
	}
	if errs[0].Rule != "unresolved_variable" {
		t.Fatalf("expected rule %q, got %q", "unresolved_variable", errs[0].Rule)
	}
}

func TestVariableRefsRule_EmptyTargets(t *testing.T) {
	rule := VariableRefsRule{}
	m := &domain.ArrowManifest{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
