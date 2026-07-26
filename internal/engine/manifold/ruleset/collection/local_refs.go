package collection

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
)

// CheckLocalRefs returns an error for every local arrow that carries no ref. A
// remote entry resolves its own ref when it is followed, but a local one is a
// path inside this repository with nothing to resolve against, so the ref has
// to be written on the path: "path: servers/cs2@v1.0.0".
func CheckLocalRefs(arrows []domain.CollectionArrow) error {
	var errs aerrors.RuleErrors
	for i, a := range arrows {
		if !a.IsLocal || a.Namespace.Ref() != "" {
			continue
		}
		errs = append(errs, aerrors.RuleError{
			Field: fmt.Sprintf("arrows[%d]", i),
			Rule:  "required_ref",
			Message: fmt.Sprintf(
				"local arrow %q must carry a ref: append @<ref> to its path",
				a.Namespace,
			),
		})
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
