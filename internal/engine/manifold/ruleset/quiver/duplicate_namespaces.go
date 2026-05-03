package quiver

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
)

// CheckDuplicateNamespaces returns an error if any resolved namespace appears more than once.
func CheckDuplicateNamespaces(arrows []domain.QuiverArrow) error {
	seen := make(map[domain.Namespace]struct{}, len(arrows))
	for _, a := range arrows {
		if _, ok := seen[a.Namespace]; ok {
			return aerrors.RuleErrors{aerrors.RuleError{
				Field:   "arrows",
				Rule:    "duplicate_namespace",
				Message: fmt.Sprintf("duplicate arrow namespace %q", a.Namespace),
			}}
		}
		seen[a.Namespace] = struct{}{}
	}
	return nil
}
