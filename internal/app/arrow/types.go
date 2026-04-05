package arrow

import (
	"errors"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrAlreadyRemoved   = errors.New("already removed")
	ErrStateViolation   = errors.New("state violation")
	ErrFetchFailed      = errors.New("fetch failed")
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrDependentsExist  = errors.New("other arrows depend on this arrow")
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
