package models

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type ArrowDetailView struct {
	Metadata   domain.Arrow
	State      domain.ArrowState
	ActiveRun  *domainRuntime.Execution
	LastReturn *domainRuntime.Return
}
