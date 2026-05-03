package step

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/models"
)

type Request struct {
	NSKey   string
	WorkDir string
	Vars    map[string]string
	// OSArch is the current host platform (e.g. "darwin/arm64") used to resolve
	// overrideable step fields to their platform-specific values.
	OSArch domain.OS
	// PID is a pre-existing OS process ID. When non-zero, signal handlers send
	// directly to this PID without needing a running process handle.
	PID int
	// Emit, if non-nil, is called by handlers to push events mid-step
	// (e.g. EventKindPID right after a process starts).
	Emit func(models.Event)
}

type Handler[S any] interface {
	Execute(
		ctx context.Context,
		req Request,
		s S,
	) error
}
