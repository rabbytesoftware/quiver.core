//go:build windows

package builder

import (
	"strings"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

func (b *Builder) Build() (process.Process, error) {
	if b.config.ShellWrap {
		joined := strings.Join(b.config.Command, " ")
		b.config.Command = []string{"cmd.exe", "/C", joined}
	}

	if err := b.config.Validate(); err != nil {
		return nil, err
	}

	var proc process.Process
	var err error

	switch b.os {
	case "windows":
		proc, err = process.NewWindowsProcess(b.ctx, b.config)
	default:
		return nil, models.ErrUnsupportedOS
	}

	if err != nil {
		return nil, err
	}

	b.manager.Register(proc)

	return proc, nil
}
