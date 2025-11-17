//go:build linux

package builder

import (
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/process"
)

func (b *Builder) Build() (process.Process, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	var proc process.Process
	var err error

	switch b.os {
	case "linux":
		proc, err = process.NewLinuxProcess(b.ctx, b.config)
	default:
		return nil, models.ErrUnsupportedOS
	}

	if err != nil {
		return nil, err
	}

	b.manager.Register(proc)

	return proc, nil
}
