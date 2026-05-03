package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestBaseIntegrityRule_NoBase(t *testing.T) {
	rule := BaseIntegrityRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"standalone": {},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestBaseIntegrityRule_LinearChain(t *testing.T) {
	rule := BaseIntegrityRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"a": {Base: "b"},
		"b": {},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid chain A→B→\"\", got: %v", errs)
	}
}

func TestBaseIntegrityRule_MissingBase(t *testing.T) {
	rule := BaseIntegrityRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"a": {Base: "missing"},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got: %v", errs)
	}
	if errs[0].Rule != "missing_base" {
		t.Errorf("expected rule missing_base, got %q", errs[0].Rule)
	}
	if errs[0].Field != "targets[a].base" {
		t.Errorf("expected field targets[a].base, got %q", errs[0].Field)
	}
}

func TestBaseIntegrityRule_CyclicBase(t *testing.T) {
	rule := BaseIntegrityRule{}
	precompiled := map[string]models.PrecompiledTarget{
		"a": {Base: "b"},
		"b": {Base: "a"},
	}
	errs := rule.Validate(&domain.Arrow{}, precompiled)
	if len(errs) == 0 {
		t.Fatal("expected at least one cyclic_base error, got none")
	}
	found := false
	for _, e := range errs {
		if e.Rule == "cyclic_base" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cyclic_base rule, got: %v", errs)
	}
}
