package models

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type InstalledVersionDTO struct {
	Ref         string            `json:"ref"`
	Version     string            `json:"version"`
	State       domain.ArrowState `json:"state"`
	InstalledAt time.Time         `json:"installed_at"`
	Constraint  string            `json:"constraint,omitempty"`
}
