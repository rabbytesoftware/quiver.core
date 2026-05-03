package runtime

import (
	"context"
	stdruntime "runtime"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/internal/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/internal/process"
)

type Process = process.Process
type Status = models.Status
type Config = models.Config

const (
	StatusPrepared = models.StatusPrepared
	StatusRunning  = models.StatusRunning
	StatusStopping = models.StatusStopping
	StatusKilling  = models.StatusKilling
	StatusFinished = models.StatusFinished
)

type Runtime interface {
	Start(
		ctx context.Context,
		config *Config,
	) (Process, error)

	SignalPID(
		ctx context.Context,
		pid int,
		sig domainstep.SignalKind,
	) error

	ProcessAlive(pid int) bool
}

func NewConfig(
	command []string,
) *Config {
	return models.NewConfig(command)
}

type runtime struct {
	os string
}

func New() (Runtime, error) {
	return newForOS(stdruntime.GOOS)
}

func newForOS(
	os string,
) (Runtime, error) {
	if !isSupportedOS(os) {
		return nil, ErrUnsupportedOS
	}
	return &runtime{os: os}, nil
}

func (r *runtime) Start(
	ctx context.Context,
	config *Config,
) (Process, error) {
	return process.New(ctx, config)
}

// signal dispatches a SignalKind to a live Process handle.
// Used internally by tests; production code uses SignalPID with a known PID.
func (r *runtime) signal(
	ctx context.Context,
	proc Process,
	sig domainstep.SignalKind,
) error {
	switch sig {
	case domainstep.SignalKindGraceful:
		return proc.Stop(ctx)
	case domainstep.SignalKindKill:
		return proc.Kill(ctx)
	case domainstep.SignalKindInterrupt:
		return proc.Interrupt(ctx)
	default:
		return ErrInvalidSignal
	}
}

func (r *runtime) SignalPID(
	ctx context.Context,
	pid int,
	sig domainstep.SignalKind,
) error {
	return process.SignalPID(ctx, pid, sig)
}

func (r *runtime) ProcessAlive(pid int) bool {
	return process.IsAlive(pid)
}

func isSupportedOS(os string) bool {
	switch os {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}
