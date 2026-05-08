package collection

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// CheckDuplicateNamespaces returns an error if any resolved namespace appears more than once.
func CheckDuplicateNamespaces(arrows []domain.CollectionArrow) error {
	seen := make(map[domain.Namespace]struct{}, len(arrows))
	var errs aerrors.RuleErrors
	for _, a := range arrows {
		if _, ok := seen[a.Namespace]; ok {
			errs = append(errs, aerrors.RuleError{
				Field:   "arrows",
				Rule:    "duplicate_namespace",
				Message: fmt.Sprintf("duplicate arrow namespace %q", a.Namespace),
			})
		}
		seen[a.Namespace] = struct{}{}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
