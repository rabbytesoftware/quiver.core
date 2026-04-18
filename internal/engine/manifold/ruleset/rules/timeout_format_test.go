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

func TestTimeoutFormatRule_EmptyTargets(t *testing.T) {
	rule := TimeoutFormatRule{}
	m := &domain.ArrowManifest{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
