package runtime

import (
	"context"
	"fmt"
	stdruntime "runtime"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/process"
)

type Runtime struct {
	manager *Manager
	os      string
}

func New() (*Runtime, error) {
	os := detectOS()
	
	if !isSupportedOS(os) {
		return nil, fmt.Errorf("%w: %s", models.ErrUnsupportedOS, os)
	}

	return &Runtime{
		manager: NewManager(),
		os:      os,
	}, nil
}

func (r *Runtime) Get(ctx context.Context, command ...string) *process.Builder {
	return process.NewBuilder(ctx, r.manager, r.os, command)
}

func (r *Runtime) GetByID(id string) (process.Process, error) {
	return r.manager.Get(id)
}

func (r *Runtime) ListAll() []process.Process {
	return r.manager.ListAll()
}

func (r *Runtime) ListByStatus(status models.Status) []process.Process {
	return r.manager.ListByStatus(status)
}

func (r *Runtime) Count() int {
	return r.manager.Count()
}

func (r *Runtime) StopAll(ctx context.Context) error {
	return r.manager.StopAll(ctx)
}

func (r *Runtime) KillAll(ctx context.Context) error {
	return r.manager.KillAll(ctx)
}

func (r *Runtime) CleanupFinished() int {
	return r.manager.CleanupFinished()
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	return r.manager.ShutdownAll(ctx)
}

func (r *Runtime) OS() string {
	return r.os
}

func detectOS() string {
	return stdruntime.GOOS
}

func isSupportedOS(os string) bool {
	switch os {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}
