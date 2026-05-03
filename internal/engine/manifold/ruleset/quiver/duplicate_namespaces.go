package quiver

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// CheckDuplicateNamespaces returns an error if any resolved namespace appears more than once.
func CheckDuplicateNamespaces(arrows []domain.QuiverArrow) error {
	seen := make(map[domain.Namespace]struct{}, len(arrows))
	for _, a := range arrows {
		if _, ok := seen[a.Namespace]; ok {
			return fmt.Errorf("ruleset: duplicate arrow namespace %q", a.Namespace)
		}
		seen[a.Namespace] = struct{}{}
	}
	return nil
}
