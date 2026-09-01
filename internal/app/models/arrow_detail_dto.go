package models

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type ArrowDetailDTO struct {
	Namespace           domain.Namespace            `json:"namespace"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description"`
	Tags                []string                    `json:"tags"`
	Variables           []domain.Variable           `json:"variables"`
	Targets             map[domain.OS]domain.Target `json:"targets"`
	InstalledAt         time.Time                   `json:"installed_at"`
	LastUsedAt          time.Time                   `json:"last_used_at"`
	InstalledConstraint string                      `json:"installed_constraint"`
	UserInstalled       bool                        `json:"user_installed"`
	State               domain.ArrowState           `json:"state"`
	ActiveRun           *domainRuntime.Execution    `json:"active_run,omitempty"`
	LastReturn          *domainRuntime.Return       `json:"last_return,omitempty"`
}
