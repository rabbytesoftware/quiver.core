package process

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
)

type ProcessManager interface {
	Register(Process)
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

func (b *Builder) Build() (Process, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	var proc Process
	var err error

	switch b.os {
	case "darwin":
		proc, err = NewDarwinProcess(b.ctx, b.config)
	case "linux":
		proc, err = NewLinuxProcess(b.ctx, b.config)
	case "windows":
		proc, err = NewWindowsProcess(b.ctx, b.config)
	default:
		return nil, models.ErrUnsupportedOS
	}

	if err != nil {
		return nil, err
	}

	b.manager.Register(proc)

	return proc, nil
}

