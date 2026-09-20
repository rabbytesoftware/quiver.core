package collection

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

// CheckArrowEntries enforces that each CollectionArrowEntry has exactly one of
// Path or Namespace set (XOR), that AUID, when set, only overrides the
// identity of a local (Path) entry and holds a single namespace segment, and
// that Path, when set, does not escape the repository or corrupt the URL it
// is substituted into.
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
		if e.AUID != "" && e.Namespace != "" {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("arrows[%d].auid", i),
				Rule:    "auid_with_namespace",
				Message: "auid may only be set on a local (path) entry, not alongside namespace",
			})
		}
		if strings.ContainsAny(e.AUID, "/@") {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("arrows[%d].auid", i),
				Rule:    "invalid_auid",
				Message: fmt.Sprintf("auid %q must not contain '/' or '@'", e.AUID),
			})
		}
		if strings.ContainsAny(e.Path, "\\?#") || strings.Contains(e.Path, "..") {
			errs = append(errs, aerrors.RuleError{
				Field:   fmt.Sprintf("arrows[%d].path", i),
				Rule:    "invalid_path",
				Message: fmt.Sprintf("path %q must not contain '..', '\\\\', '?', or '#'", e.Path),
			})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
