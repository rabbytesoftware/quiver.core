package portable_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func assertExecutable(
	t *testing.T,
	path string,
	want string,
) {
	t.Helper()

	assert.Equal(t, want, unpacktest.ReadString(t, path))
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestHandler_Execute_Executable(t *testing.T) {
	testCases := []struct {
		name  string
		magic string
	}{
		{name: "elf", magic: "\x7fELF"},
		{name: "mach-o big endian", magic: "\xfe\xed\xfa\xcf"},
		{name: "mach-o little endian", magic: "\xcf\xfa\xed\xfe"},
		{name: "mach-o universal", magic: "\xca\xfe\xba\xbe"},
		{name: "pe", magic: "MZ"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			body := tc.magic + "payload"
			from := writeFile(t, filepath.Join(workDir, "dl", "tool"), []byte(body))

			err := runPortable(t, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", "")

			require.NoError(t, err)
			assertExecutable(t, filepath.Join(workDir, "bin", "tool"), body)
			assert.NoFileExists(t, from)
			assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
		})
	}
}

func TestHandler_Execute_ExecutableReplacesExistingOutput(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "dl", "tool"), []byte(elfExecutable))
	out := writeFile(t, filepath.Join(workDir, "bin", "tool"), []byte("old"))
	require.NoError(t, os.Chmod(out, 0o444))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "dl/tool", "bin", "")

	require.NoError(t, err)
	assertExecutable(t, out, elfExecutable)
}

func TestHandler_Execute_ExecutableAlreadyInPlaceIsKept(t *testing.T) {
	workDir := t.TempDir()
	from := writeFile(t, filepath.Join(workDir, "bin", "tool"), []byte(elfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bin/tool", "bin", "")

	require.NoError(t, err)
	assertExecutable(t, from, elfExecutable)
}

func TestHandler_Execute_ExecutableFailures(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		setup    func(t *testing.T, workDir string)
		wantErr  error
		wantMsg  string
	}{
		{
			name:     "larger than the limit",
			maxBytes: 4,
			setup:    func(t *testing.T, workDir string) {},
			wantErr:  unpack.ErrTooLarge,
			wantMsg:  "bytes, exceeds the 4-byte limit",
		},
		{
			name:     "destination is a file",
			maxBytes: unpacktest.TestMaxBytes,
			setup: func(t *testing.T, workDir string) {
				writeFile(t, filepath.Join(workDir, "bin"), []byte("x"))
			},
			wantMsg: "portable: create",
		},
		{
			name:     "output is a non-empty directory",
			maxBytes: unpacktest.TestMaxBytes,
			setup: func(t *testing.T, workDir string) {
				writeFile(t, filepath.Join(workDir, "bin", "tool", "x"), []byte("x"))
			},
			wantMsg: "portable: copy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := writeFile(t, filepath.Join(workDir, "dl", "tool"), []byte(elfExecutable))
			tc.setup(t, workDir)

			h := portable.NewHandler(tc.maxBytes)
			s := domainstep.NewPortableStep("portable", "dl/tool", "bin", "", true)
			err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.NotContains(t, err.Error(), "uncompressed")
			assert.FileExists(t, from)
		})
	}
}
