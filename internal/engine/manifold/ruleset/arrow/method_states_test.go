package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestMethodStatesRule_Valid(t *testing.T) {
	rule := MethodStatesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Methods: map[string]domain.Method{
					"start": {AvailableIn: []domain.ArrowState{domain.ArrowStateReady}},
					"stop":  {AvailableIn: []domain.ArrowState{domain.ArrowStateRunning}},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestMethodStatesRule_InvalidState(t *testing.T) {
	rule := MethodStatesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Methods: map[string]domain.Method{
					"action": {AvailableIn: []domain.ArrowState{"invalid_state"}},
				},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid state, got none")
	}
	if errs[0].Rule != "invalid_state" {
		t.Fatalf("expected rule %q, got %q", "invalid_state", errs[0].Rule)
	}
}

func TestMethodStatesRule_NoMethods(t *testing.T) {
	rule := MethodStatesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for target with no methods, got: %v", errs)
	}
}

func TestMethodStatesRule_EmptyTargets(t *testing.T) {
	rule := MethodStatesRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
