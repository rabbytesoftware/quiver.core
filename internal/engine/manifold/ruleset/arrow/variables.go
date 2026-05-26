package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

type VariablesRule struct{}

func (VariablesRule) Name() string { return "variables" }

func (VariablesRule) Validate(
	m *domain.Arrow,
	_ map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors
	names := make([]string, 0, len(m.Variables))

	for i, v := range m.Variables {
		names = append(names, v.Name)

		if err := v.Validate(); err != nil {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("variables[%d]", i),
				Rule:    "invalid_variable",
				Message: err.Error(),
			})
			continue
		}

		if v.Type.IsSelect() && len(v.Values) == 0 {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("variables[%d].values", i),
				Rule:    "missing_values",
				Message: fmt.Sprintf("select variable %q must have at least one value", v.Name),
			})
		}
	}

	errs = append(errs, checkDuplicates(names, "variables", "duplicate_name")...)
	if len(errs) == 0 {
		return nil
	}
	return errs
}
