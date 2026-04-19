package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestTimeoutFormatRule_Valid(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "echo ok", false, "30s", true),
					},
					Uninstall: step.StepList{
						step.NewRunStep("uninstall", "echo bye", false, "5m", true),
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestTimeoutFormatRule_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{
						step.NewRunStep("install", "echo ok", false, "5minutes", true),
					},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid timeout format, got none")
	}
	if errs[0].Rule != "invalid_timeout" {
		t.Fatalf("expected rule %q, got %q", "invalid_timeout", errs[0].Rule)
	}
}

func TestTimeoutFormatRule_EmptyTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "", true)},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty timeout, got: %v", errs)
	}
}

func TestTimeoutFormatRule_DependenciesStep_NoTimeout(t *testing.T) {
	// DependenciesStep has no timeout field; extractStepTimeout should return ""
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{step.NewDependenciesStep("install deps")},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for DependenciesStep (no timeout), got: %v", errs)
	}
}

func TestTimeoutFormatRule_MethodSteps_InvalidTimeout(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
					Uninstall: step.StepList{},
				},
				Methods: map[string]domain.Method{
					"check": {
						AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
						Steps:       step.StepList{step.NewRunStep("check", "check.sh", false, "2h", true)},
					},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid method step timeout, got none")
	}
	if errs[0].Rule != "invalid_timeout" {
		t.Fatalf("expected rule %q, got %q", "invalid_timeout", errs[0].Rule)
	}
}

func TestTimeoutFormatRule_EmptyTargets(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
