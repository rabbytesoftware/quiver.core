package builder

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

type ProcessManager interface {
	Register(process.Process)
	Unregister(string)
}

type Builder struct {
	ctx     context.Context
	manager ProcessManager
	os      string
	config  *models.Config
}

func NewBuilder(ctx context.Context, manager ProcessManager, os string, command []string) *Builder {
	return &Builder{
		ctx:     ctx,
		manager: manager,
		os:      os,
		config:  models.NewConfig(command),
	}
}

func (b *Builder) WithWorkDir(dir string) *Builder {
	b.config.WorkDir = dir
	return b
}

func (b *Builder) WithEnv(env map[string]string) *Builder {
	for k, v := range env {
		b.config.Env[k] = v
	}
	return b
}

func (b *Builder) WithEnvVar(key, value string) *Builder {
	b.config.Env[key] = value
	return b
}

func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
	b.config.Timeout = timeout
	return b
}

func (b *Builder) WithBufferSize(size int) *Builder {
	b.config.BufferSize = size
	return b
}

func (b *Builder) WithKillTimeout(timeout time.Duration) *Builder {
	b.config.KillTimeout = timeout
	return b
}

func (b *Builder) WithStopTimeout(timeout time.Duration) *Builder {
	b.config.StopTimeout = timeout
	return b
}

// Build is implemented in OS-specific files to avoid compilation issues with build tags
// See: builder_darwin.go, builder_linux.go, builder_windows.go
