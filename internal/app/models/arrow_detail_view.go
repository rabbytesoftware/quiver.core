package models

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type ArrowDetailView struct {
	Metadata   domain.Arrow
	State      domain.ArrowState
	ActiveRun  *domainRuntime.Execution
	LastReturn *domainRuntime.Return
	// LastVersionCheckAt is the read model's throttle stamp for the passive
	// version-drift check, carried alongside Metadata so a caller can decide
	// staleness without a second query. Zero when never checked.
	LastVersionCheckAt time.Time
}
