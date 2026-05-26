package process

import (
	"context"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

type Process interface {
	ID() string
	PID() int
	Status() models.Status
	Done() <-chan struct{}

	Stop(ctx context.Context) error
	Kill(ctx context.Context) error
	Interrupt(ctx context.Context) error

	Wait(ctx context.Context) error
	Close() error
	ExitCode() int

	Output() string
	Error() string
	StreamOutput() <-chan string
	StreamError() <-chan string
}

func New(
	ctx context.Context,
	config *models.Config,
) (Process, error) {
	return newProcess(ctx, config)
}

func SignalPID(
	ctx context.Context,
	pid int,
	sig domainstep.SignalKind,
) error {
	return signalPID(ctx, pid, sig)
}

// IsAlive reports whether a process with the given PID is currently running.
// Returns false on Windows (safe default: treat as dead, trigger recovery).
func IsAlive(pid int) bool {
	return isAlive(pid)
}
