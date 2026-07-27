package models

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionDTO is one namespace@ref row of a catalog entry. Ref and
// Version are not the same fact and neither can be dropped for the other:
// Version is the ref the row is filed under and is always present, while Ref is
// the stamp an _install lifecycle leaves behind and is empty until one succeeds
// and again after an _uninstall. A row that is in the catalog but not on disk
// therefore has a Version and no Ref.
type InstalledVersionDTO struct {
	Ref         string            `json:"ref"`
	Version     string            `json:"version"`
	State       domain.ArrowState `json:"state"`
	InstalledAt time.Time         `json:"installed_at"`
	Constraint  string            `json:"constraint,omitempty"`
}
