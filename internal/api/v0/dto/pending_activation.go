package dto

import (
	"time"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// PendingActivationDTO is a staged binary waiting for a daemon restart. The
// staged path and digest stay inside the daemon.
type PendingActivationDTO struct {
	Version  string `json:"version" yaml:"version"`
	StagedAt string `json:"staged_at" yaml:"staged_at"`
}

// PendingActivationDTOFrom maps a staged activation, or nil when nothing is
// staged.
func PendingActivationDTOFrom(
	p *domainRuntime.PendingActivation,
) *PendingActivationDTO {
	if p == nil {
		return nil
	}
	return &PendingActivationDTO{
		Version:  p.Version,
		StagedAt: p.StagedAt.UTC().Format(time.RFC3339),
	}
}
