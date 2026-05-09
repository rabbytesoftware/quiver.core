package collection

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

// CheckArrowEntries enforces that each CollectionArrowEntry has exactly one of
// Path or Namespace set (XOR).
func CheckArrowEntries(entries []domain.CollectionArrowEntry) error {
	var errs aerrors.RuleErrors
	for i, e := range entries {
		if e.Path != "" && e.Namespace != "" {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("arrows[%d]", i),
				Rule:    "exclusive_fields",
				Message: "arrow entry must have either path or namespace, not both",
			})
		}
		if e.Path == "" && e.Namespace == "" {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("arrows[%d]", i),
				Rule:    "required_field",
				Message: "arrow entry must have either path or namespace",
			})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
