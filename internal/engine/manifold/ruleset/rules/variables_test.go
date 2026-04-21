package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestVariablesRule_Valid(t *testing.T) {
	rule := VariablesRule{}
	m := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: "MY_VAR"},
			{Name: "OTHER_VAR"},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestVariablesRule_EmptyName(t *testing.T) {
	rule := VariablesRule{}
	m := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: ""},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for empty variable name, got none")
	}
	if errs[0].Rule != "invalid_variable" {
		t.Fatalf("expected rule %q, got %q", "invalid_variable", errs[0].Rule)
	}
}

func TestVariablesRule_SelectWithoutValues(t *testing.T) {
	rule := VariablesRule{}
	vt := domain.VariableTypeSelect
	m := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: "CHOICE", Type: vt, Values: nil},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for select variable without values, got none")
	}
	found := false
	for _, e := range errs {
		if e.Rule == "missing_values" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rule %q in errors, got: %v", "missing_values", errs)
	}
}

func TestVariablesRule_DuplicateName(t *testing.T) {
	rule := VariablesRule{}
	m := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: "MY_VAR"},
			{Name: "MY_VAR"},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for duplicate variable name, got none")
	}
	found := false
	for _, e := range errs {
		if e.Rule == "duplicate_name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rule %q in errors, got: %v", "duplicate_name", errs)
	}
}

func TestVariablesRule_NoVariables(t *testing.T) {
	rule := VariablesRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty variables, got: %v", errs)
	}
}
