//go:build darwin

package builder

import (
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

func (b *Builder) Build() (process.Process, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	var proc process.Process
	var err error

	switch b.os {
	case "darwin":
		proc, err = process.NewDarwinProcess(b.ctx, b.config)
	default:
		return nil, models.ErrUnsupportedOS
	}

	if err != nil {
		return nil, err
	}

	b.manager.Register(proc)

	return proc, nil
}
