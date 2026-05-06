package quiver

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// CheckArrowEntries enforces that each QuiverArrowEntry has exactly one of
// Path or Namespace set (XOR).
func CheckArrowEntries(entries []domain.QuiverArrowEntry) error {
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
