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

func TestHandler_Execute_AppImage(t *testing.T) {
	workDir := t.TempDir()
	from := writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	appDir := filepath.Join(workDir, "bruno")
	assert.Equal(t, "#!/bin/sh\n", unpacktest.ReadString(t, filepath.Join(appDir, "AppRun")))
	assert.Contains(t, unpacktest.ReadString(t, filepath.Join(appDir, unpack.LauncherName)), "'--no-sandbox'")
	_, err = os.Lstat(filepath.Join(appDir, ".DirIcon"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{{
		Name:  "bruno",
		Entry: "bruno/.quiver-run",
		Icon:  "bruno/usr/share/icons/hicolor/256x256/apps/bruno.png",
	}}}, readRecord(t, workDir))
	assert.NoFileExists(t, from)

	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(filepath.Join(appDir, unpack.LauncherName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

func TestHandler_Execute_ExpandsVariablesAndOverrides(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "dl", "tool"), []byte(elfExecutable))

	h := portable.NewHandler(unpacktest.TestMaxBytes)
	s := domainstep.NewPortableStep("portable", "missing", "${DIR}", "", true)
	s.From.OSArch = map[string]string{"linux/amd64": "dl/tool"}
	req := wizstep.Request{WorkDir: workDir, OSArch: "linux/amd64", Vars: map[string]string{"DIR": "bin"}}
	err := h.Execute(context.Background(), req, s)

	require.NoError(t, err)
	assert.Equal(t, elfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "bin", "tool")))
}

func TestHandler_Execute_UnknownFormat(t *testing.T) {
	workDir := t.TempDir()
	from := writeFile(t, filepath.Join(workDir, "notes.txt"), []byte("just some text"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "notes.txt", ".", "")

	require.ErrorIs(t, err, portable.ErrUnknownFormat)
	assert.Equal(t, "portable: unknown format: "+from, err.Error())
	assert.FileExists(t, from)
}

func TestHandler_Execute_InvalidTimeout(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "tool"), []byte(elfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool", "bin", "soon")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable: invalid timeout")
}

func TestHandler_Execute_SourceOutsideWorkdirKept(t *testing.T) {
	workDir := t.TempDir()
	from := writeFile(t, filepath.Join(t.TempDir(), "tool"), []byte(elfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, from, "bin", "")

	require.NoError(t, err)
	assert.FileExists(t, from)
	assert.Equal(t, elfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "bin", "tool")))
}

func TestHandler_Execute_DestinationOutsideWorkdirRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	to := t.TempDir()
	from := writeAppImage(t, workDir, "bruno.AppImage", appImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", to, "")

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(to, "bruno", unpack.LauncherName))
	assert.NoFileExists(t, filepath.Join(workDir, domain.PortableRecordFile))
	assert.NoFileExists(t, from)
}

func TestHandler_Execute_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T, workDir string) string
		to      string
		timeout string
		wantErr error
		wantMsg string
	}{
		{
			name:    "missing input",
			setup:   func(t *testing.T, workDir string) string { return "absent" },
			to:      "out",
			wantErr: os.ErrNotExist,
			wantMsg: "portable: open",
		},
		{
			name: "input is a directory",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.Mkdir(filepath.Join(workDir, "srcdir"), 0o755))
				return "srcdir"
			},
			to:      "out",
			wantMsg: "unpack: read ",
		},
		{
			name: "expired timeout",
			setup: func(t *testing.T, workDir string) string {
				writeFile(t, filepath.Join(workDir, "a.tar"), unpacktest.HelloTar(t))
				return "a.tar"
			},
			to:      "out",
			timeout: "1ns",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "archive destination is a file",
			setup: func(t *testing.T, workDir string) string {
				writeFile(t, filepath.Join(workDir, "a.tar"), unpacktest.HelloTar(t))
				writeFile(t, filepath.Join(workDir, "out"), []byte("x"))
				return "a.tar"
			},
			to:      "out",
			wantMsg: "unpack: create",
		},
		{
			name: "corrupt archive",
			setup: func(t *testing.T, workDir string) string {
				writeFile(t, filepath.Join(workDir, "a.tar.gz"), []byte("not gzip at all"))
				return "a.tar.gz"
			},
			to:      "out",
			wantMsg: "unpack",
		},
		{
			name: "record path is a directory",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))
				writeFile(t, filepath.Join(workDir, "Foo.zip"), zipBytes(t, map[string]string{"Foo.app/Contents/MacOS/foo": "bin"}))
				return "Foo.zip"
			},
			to:      ".",
			wantMsg: "portable: read record",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			from := tc.setup(t, workDir)

			req := wizstep.Request{WorkDir: workDir, OSArch: "darwin/arm64"}
			err := runPortable(t, req, from, tc.to, tc.timeout)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
