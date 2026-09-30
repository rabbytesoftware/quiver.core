package confidence

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

// Assessment is the verdict on a set of picks: the confidence level, the
// warnings behind it, and the picks that stay in the manifest.
type Assessment struct {
	Level    Confidence
	Warnings []string
	Picks    map[domain.OS]picker.Pick
}

// KeepsAny reports whether at least one platform survived the assessment.
func (a Assessment) KeepsAny() bool {
	return len(a.Picks) > 0
}
