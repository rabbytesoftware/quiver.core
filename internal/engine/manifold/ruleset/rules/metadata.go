package rules

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// MetadataRule enforces length limits on manifest metadata fields.
type MetadataRule struct{}

func (MetadataRule) Name() string { return "metadata" }

func (MetadataRule) Validate(
	m *domain.Arrow,
	_ map[string]models.PrecompiledTarget,
) aerrors.RuleErrors {
	var errs aerrors.RuleErrors

	if len(m.Name) == 0 {
		errs = append(errs, aerrors.RuleError{
			Field:   "metadata.name",
			Rule:    "required",
			Message: "name is required",
		})
	} else if len(m.Name) > domain.MaxNameLength {
		errs = append(errs, aerrors.RuleError{
			Field:   "metadata.name",
			Rule:    "max_length",
			Message: fmt.Sprintf("name must not exceed %d characters", domain.MaxNameLength),
		})
	}

	if len(m.Description) > domain.MaxDescriptionLength {
		errs = append(errs, aerrors.RuleError{
			Field:   "metadata.description",
			Rule:    "max_length",
			Message: fmt.Sprintf("description must not exceed %d characters", domain.MaxDescriptionLength),
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}
