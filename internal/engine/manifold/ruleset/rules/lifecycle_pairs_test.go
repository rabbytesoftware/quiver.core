package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

func TestLifecyclePairsRule_Valid(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install:   step.StepList{},
					Uninstall: step.StepList{},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestLifecyclePairsRule_MissingUninstall(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{step.RunStep{}},
					// Uninstall absent
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing uninstall, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_StopWithoutExecute(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Stop: step.StepList{step.RunStep{}},
					// Execute absent — stop without execute is invalid
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for missing stop, got none")
	}
	if errs[0].Rule != "missing_pair" {
		t.Fatalf("expected rule %q, got %q", "missing_pair", errs[0].Rule)
	}
}

func TestLifecyclePairsRule_EmptyTargets(t *testing.T) {
	rule := LifecyclePairsRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
