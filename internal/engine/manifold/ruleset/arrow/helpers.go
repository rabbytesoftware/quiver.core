package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

func checkDuplicates(
	names []string,
	field string,
	rule string,
) aerrors.RuleErrors {
	seen := make(map[string]bool, len(names))
	reported := make(map[string]bool)
	var errs aerrors.RuleErrors
	for _, name := range names {
		isDup := seen[name] && !reported[name]
		seen[name] = true
		if !isDup {
			continue
		}
		reported[name] = true
		errs = append(errs, aerrors.RuleError{
			Field:   field,
			Rule:    rule,
			Message: fmt.Sprintf("duplicate name %q", name),
		})
	}
	return errs
}
