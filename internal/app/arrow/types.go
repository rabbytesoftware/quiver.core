package arrow

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type ArrowListDTO struct {
	Namespace   domain.Namespace  `json:"namespace"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	State       domain.ArrowState `json:"state"`
	Tags        []string          `json:"tags"`
	Removed     bool              `json:"removed"`
}

type ArrowDetailDTO struct {
	Namespace            domain.Namespace         `json:"namespace"`
	Manifest             domain.ArrowManifest     `json:"manifest"`
	State                domain.ArrowState        `json:"state"`
	Removed              bool                     `json:"removed"`
	ActiveRun            *domainRuntime.RunRecord `json:"active_run,omitempty"`
	LastReturn           *domainRuntime.Return    `json:"last_return,omitempty"`
	IndirectDependencies []domain.Namespace       `json:"indirect_dependencies,omitempty"`
}
