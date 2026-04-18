package aerrors_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

func TestRuleError_Error_ContainsFieldAndRule(t *testing.T) {
	ae := aerrors.RuleError{
		Field:   "targets[linux/*].lifecycle.install",
		Rule:    "missing_pair",
		Message: "install and uninstall must both be defined or both be empty",
	}

	got := ae.Error()

	if got == "" {
		t.Fatal("expected non-empty error string")
	}
	if !strings.Contains(got, ae.Field) {
		t.Errorf("Error() = %q, want it to contain field %q", got, ae.Field)
	}
	if !strings.Contains(got, ae.Rule) {
		t.Errorf("Error() = %q, want it to contain rule %q", got, ae.Rule)
	}
}

func TestRuleError_Unwrap_IsErrInvalidManifest(t *testing.T) {
	ae := aerrors.RuleError{
		Field:   "targets[*].lifecycle.install",
		Rule:    "missing_pair",
		Message: "some message",
	}

	if !errors.Is(ae, aerrors.ErrInvalidManifest) {
		t.Errorf("expected RuleError to unwrap to ErrInvalidManifest")
	}
}

func TestRuleErrors_Error_NonEmpty(t *testing.T) {
	errs := aerrors.RuleErrors{
		{Field: "targets[*].lifecycle.install", Rule: "missing_pair", Message: "msg1"},
		{Field: "targets[*].lifecycle.execute", Rule: "missing_pair", Message: "msg2"},
	}

	got := errs.Error()

	if got == "" {
		t.Fatal("expected non-empty joined error string")
	}
}

func TestRuleErrors_Unwrap_ReturnsErrInvalidManifest(t *testing.T) {
	errs := aerrors.RuleErrors{
		{Field: "targets[*]", Rule: "some_rule", Message: "some message"},
	}

	if !errors.Is(errs, aerrors.ErrInvalidManifest) {
		t.Errorf("expected RuleErrors to unwrap to ErrInvalidManifest via errors.Is")
	}
}

func TestRuleErrors_Unwrap_ReturnsSliceOfErrors(t *testing.T) {
	errs := aerrors.RuleErrors{
		{Field: "f1", Rule: "r1", Message: "m1"},
		{Field: "f2", Rule: "r2", Message: "m2"},
	}

	unwrapped := errs.Unwrap()

	if len(unwrapped) != 2 {
		t.Fatalf("Unwrap() returned %d errors, want 2", len(unwrapped))
	}
	for i, err := range unwrapped {
		if err == nil {
			t.Errorf("Unwrap()[%d] is nil", i)
		}
	}
}
