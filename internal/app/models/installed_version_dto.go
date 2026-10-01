package models

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// InstalledVersionDTO is one namespace@selector row of a catalog entry. Ref is
// the selector the row is filed under and is always present; ResolvedRef is
// the ref that selector installed, empty until it resolved one.
type InstalledVersionDTO struct {
	Ref         string            `json:"ref"`
	ResolvedRef string            `json:"resolved_ref"`
	State       domain.ArrowState `json:"state"`
	InstalledAt time.Time         `json:"installed_at"`
	LastUsedAt  time.Time         `json:"last_used_at"`
}
