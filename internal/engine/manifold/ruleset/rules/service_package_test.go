package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func makeResolvedInstallTarget() domain.Target {
	return domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install: step.StepList{
				step.NewRunStep("install", "echo ok", false, "10s", true),
			},
			Uninstall: step.StepList{},
		},
	}
}

func makeResolvedServiceTarget() domain.Target {
	return domain.Target{
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
			Uninstall: step.StepList{},
		},
	}
}

func TestServicePackageRule_Valid_AllPackage(t *testing.T) {
	rule := ServicePackageRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OS("linux/amd64"):  makeResolvedInstallTarget(),
			domain.OS("darwin/arm64"): makeResolvedInstallTarget(),
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for all-package targets, got: %v", errs)
	}
}

func TestServicePackageRule_Valid_AllService(t *testing.T) {
	rule := ServicePackageRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OS("linux/amd64"):  makeResolvedServiceTarget(),
			domain.OS("darwin/arm64"): makeResolvedServiceTarget(),
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for all-service targets, got: %v", errs)
	}
}

func TestServicePackageRule_MixedKind(t *testing.T) {
	rule := ServicePackageRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OS("linux/amd64"):  makeResolvedServiceTarget(),
			domain.OS("darwin/arm64"): makeResolvedInstallTarget(),
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for mixed service/package targets, got none")
	}
	if errs[0].Rule != "mixed_kind" {
		t.Fatalf("expected rule %q, got %q", "mixed_kind", errs[0].Rule)
	}
}

func TestServicePackageRule_EmptyTargets(t *testing.T) {
	rule := ServicePackageRule{}
	errs := rule.Validate(&domain.Arrow{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
