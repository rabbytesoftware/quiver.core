package msi

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

const (
	exitSuccess        = 0
	exitRebootRequired = 3010
	logTailLines       = 20
	utf16BOM           = "\xff\xfe"
)

type invocation struct {
	msi    string
	target string
	log    string
}

type runner func(
	ctx context.Context,
	inv invocation,
) (int, error)

func adminInstall(
	ctx context.Context,
	run runner,
	msi string,
	target string,
	log string,
) error {
	abs, err := filepath.Abs(msi)
	if err != nil {
		return fmt.Errorf("unpack: msi %s: %w", msi, err)
	}

	code, err := run(ctx, invocation{msi: abs, target: target, log: log})
	if err != nil {
		return fmt.Errorf("unpack: msi %s: %w", abs, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("unpack: msi %s: %w", abs, err)
	}
	if code == exitSuccess || code == exitRebootRequired {
		return nil
	}

	return fmt.Errorf("unpack: msi %s: exit code %d: %w: %s", abs, code, models.ErrMsiexecFailed, logTail(log))
}

// commandLine is passed to Windows verbatim: msiexec parses PROPERTY="value"
// itself, and the quoting Go applies to a lone argument breaks paths with
// spaces there.
func (inv invocation) commandLine() string {
	return fmt.Sprintf(`msiexec /a "%s" /qn TARGETDIR="%s" /lie "%s"`, inv.msi, inv.target, inv.log)
}

func logTail(
	path string,
) string {
	data, err := os.ReadFile(path) // #nosec G304 -- log file Quiver asked msiexec to write in its own temp dir
	if err != nil {
		return "no msiexec log"
	}

	lines := strings.FieldsFunc(decodeLog(data), func(r rune) bool { return r == '\n' || r == '\r' })

	return strings.Join(lines[max(0, len(lines)-logTailLines):], " | ")
}

func decodeLog(
	data []byte,
) string {
	if !bytes.HasPrefix(data, []byte(utf16BOM)) {
		return string(data)
	}

	body := data[len(utf16BOM):]
	units := make([]uint16, len(body)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(body[2*i:])
	}

	return string(utf16.Decode(units))
}
