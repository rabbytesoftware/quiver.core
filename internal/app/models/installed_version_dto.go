package models

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionDTO is one namespace@ref row of a catalog entry. Ref is the
// ref the row is filed under and is always present; whether that ref is on disk
// is State and InstalledAt's to say, not a second ref field's.
type InstalledVersionDTO struct {
	Ref         string            `json:"ref"`
	State       domain.ArrowState `json:"state"`
	InstalledAt time.Time         `json:"installed_at"`
	Constraint  string            `json:"constraint,omitempty"`
}
