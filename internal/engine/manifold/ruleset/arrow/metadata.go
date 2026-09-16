package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
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

	if len(m.Readme) > domain.MaxReadmeLength {
		errs = append(errs, aerrors.RuleError{
			Field:   "readme",
			Rule:    "max_length",
			Message: fmt.Sprintf("readme must not exceed %d characters", domain.MaxReadmeLength),
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}
