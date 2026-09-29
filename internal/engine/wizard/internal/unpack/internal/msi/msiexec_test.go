package msi

import (
	"context"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func utf16Log(
	text string,
) []byte {
	data := []byte(utf16BOM)
	for _, unit := range utf16.Encode([]rune(text)) {
		data = binary.LittleEndian.AppendUint16(data, unit)
	}

	return data
}

func TestInvocation_CommandLine(t *testing.T) {
	inv := invocation{
		msi:    `C:\Users\Jane Doe\work\.tool.download.msi`,
		target: `C:\Users\Jane Doe\AppData\Local\Temp\quiver-msi-1\image`,
		log:    `C:\Users\Jane Doe\AppData\Local\Temp\quiver-msi-1\msiexec.log`,
	}

	assert.Equal(t,
		`msiexec /a "C:\Users\Jane Doe\work\.tool.download.msi" /qn `+
			`TARGETDIR="C:\Users\Jane Doe\AppData\Local\Temp\quiver-msi-1\image" `+
			`/lie "C:\Users\Jane Doe\AppData\Local\Temp\quiver-msi-1\msiexec.log"`,
		inv.commandLine())
}

func TestAdminInstall_ExitCodes(t *testing.T) {
	testCases := []struct {
		name    string
		code    int
		wantErr error
	}{
		{name: "success", code: exitSuccess},
		{name: "success pending reboot", code: exitRebootRequired},
		{name: "fatal error", code: 1603, wantErr: models.ErrMsiexecFailed},
		{name: "package cannot be opened", code: 1619, wantErr: models.ErrMsiexecFailed},
		{name: "another install in progress", code: 1618, wantErr: models.ErrMsiexecFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var seen invocation
			run := func(_ context.Context, inv invocation) (int, error) {
				seen = inv
				return tc.code, nil
			}

			err := adminInstall(context.Background(), run, "setup.msi", "image", "log")

			assert.Equal(t, invocation{msi: seen.msi, target: "image", log: "log"}, seen)
			assert.True(t, filepath.IsAbs(seen.msi))
			assert.Equal(t, "setup.msi", filepath.Base(seen.msi))
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestLogTail(t *testing.T) {
	lines := make([]string, 0, logTailLines+5)
	for i := range logTailLines + 5 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	long := strings.Join(lines, "\r\n")

	testCases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "utf-16 log", data: utf16Log("Error 1316.\r\nInstall failed é\r\n"), want: "Error 1316. | Install failed é"},
		{name: "plain log", data: []byte("MSI (c) ok\nError 2203.\n"), want: "MSI (c) ok | Error 2203."},
		{name: "only the tail is kept", data: []byte(long), want: strings.Join(lines[5:], " | ")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "msiexec.log"), tc.data)

			assert.Equal(t, tc.want, logTail(path))
		})
	}
}

func TestLogTail_MissingLog(t *testing.T) {
	assert.NotEmpty(t, logTail(filepath.Join(t.TempDir(), "missing.log")))
}
