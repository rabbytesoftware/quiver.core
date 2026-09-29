package portable_test

import (
	"compress/gzip"
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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func TestHandler_Execute_AppImage(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteAppImage(t, workDir, "bruno.AppImage", unpacktest.AppImageEntries("bruno"))

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
	unpacktest.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(unpacktest.ElfExecutable))

	h := portable.NewHandler(unpacktest.TestMaxBytes)
	s := domainstep.NewPortableStep("portable", "missing", "${DIR}", "", true)
	s.From.OSArch = map[string]string{"linux/amd64": "dl/tool"}
	req := wizstep.Request{WorkDir: workDir, OSArch: "linux/amd64", Vars: map[string]string{"DIR": "bin"}}
	err := h.Execute(context.Background(), req, s)

	require.NoError(t, err)
	assert.Equal(t, unpacktest.ElfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "bin", "tool")))
}

func TestHandler_Execute_UnknownFormat(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteFile(t, filepath.Join(workDir, "notes.txt"), []byte("just some text"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "notes.txt", ".", "")

	require.ErrorIs(t, err, portable.ErrUnknownFormat)
	assert.FileExists(t, from)
}

func TestHandler_Execute_InvalidTimeout(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool"), []byte(unpacktest.ElfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool", "bin", "soon")

	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(workDir, "bin"))
}

func TestHandler_Execute_SourceOutsideWorkdirKept(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteFile(t, filepath.Join(t.TempDir(), "tool"), []byte(unpacktest.ElfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, from, "bin", "")

	require.NoError(t, err)
	assert.FileExists(t, from)
	assert.Equal(t, unpacktest.ElfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "bin", "tool")))
}

func TestHandler_Execute_DestinationOutsideWorkdirRecordsNothing(t *testing.T) {
	workDir := t.TempDir()
	to := t.TempDir()
	from := unpacktest.WriteAppImage(t, workDir, "bruno.AppImage", unpacktest.AppImageEntries("bruno"))

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
		check   func(t *testing.T, workDir string)
	}{
		{
			name:    "missing input",
			setup:   func(t *testing.T, workDir string) string { return "absent" },
			to:      "out",
			wantErr: os.ErrNotExist,
		},
		{
			name: "input is a directory",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.Mkdir(filepath.Join(workDir, "srcdir"), 0o755))
				return "srcdir"
			},
			to: "out",
			check: func(t *testing.T, workDir string) {
				assert.NoDirExists(t, filepath.Join(workDir, "out"))
			},
		},
		{
			name: "expired timeout",
			setup: func(t *testing.T, workDir string) string {
				unpacktest.WriteFile(t, filepath.Join(workDir, "a.tar"), unpacktest.HelloTar(t))
				return "a.tar"
			},
			to:      "out",
			timeout: "1ns",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "archive destination is a file",
			setup: func(t *testing.T, workDir string) string {
				unpacktest.WriteFile(t, filepath.Join(workDir, "a.tar"), unpacktest.HelloTar(t))
				unpacktest.WriteFile(t, filepath.Join(workDir, "out"), []byte("x"))
				return "a.tar"
			},
			to: "out",
			check: func(t *testing.T, workDir string) {
				assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(workDir, "out")))
			},
		},
		{
			name: "corrupt archive",
			setup: func(t *testing.T, workDir string) string {
				unpacktest.WriteFile(t, filepath.Join(workDir, "a.tar.gz"), []byte("not gzip at all"))
				return "a.tar.gz"
			},
			to:      "out",
			wantErr: gzip.ErrHeader,
		},
		{
			name: "record path is a directory",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))
				unpacktest.WriteFile(t, filepath.Join(workDir, "Foo.zip"), unpacktest.ZipFiles(t, map[string]string{"Foo.app/Contents/MacOS/foo": "bin"}))
				return "Foo.zip"
			},
			to: ".",
			check: func(t *testing.T, workDir string) {
				assert.DirExists(t, filepath.Join(workDir, domain.PortableRecordFile, "x"))
				assert.DirExists(t, filepath.Join(workDir, "Foo.app"))
			},
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
			if tc.check != nil {
				tc.check(t, workDir)
			}
		})
	}
}

func TestHandler_Execute_RerunReplacesRecordEntry(t *testing.T) {
	workDir := t.TempDir()
	req := wizstep.Request{WorkDir: workDir}

	for range 2 {
		unpacktest.WriteAppImage(t, workDir, "bruno.AppImage", unpacktest.AppImageEntries("bruno"))
		require.NoError(t, runPortable(t, req, "bruno.AppImage", ".", ""))
	}
	unpacktest.WriteAppImage(t, workDir, "zed.AppImage", unpacktest.AppImageEntries("zed"))
	require.NoError(t, runPortable(t, req, "zed.AppImage", ".", ""))

	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{
		{Name: "bruno", Entry: "bruno/.quiver-run", Icon: "bruno/usr/share/icons/hicolor/256x256/apps/bruno.png"},
		{Name: "zed", Entry: "zed/.quiver-run", Icon: "zed/usr/share/icons/hicolor/256x256/apps/zed.png"},
	}}, readRecord(t, workDir))
}

func TestHandler_Execute_MalformedRecordReplaced(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, domain.PortableRecordFile), []byte("{not json"))
	unpacktest.WriteAppImage(t, workDir, "bruno.AppImage", unpacktest.AppImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	assert.Equal(t, domain.PortableRecord{Apps: []domain.PortableApp{
		{Name: "bruno", Entry: "bruno/.quiver-run", Icon: "bruno/usr/share/icons/hicolor/256x256/apps/bruno.png"},
	}}, readRecord(t, workDir))
}

func TestHandler_Execute_ExecutableTakesTheStepName(t *testing.T) {
	workDir := t.TempDir()
	from := unpacktest.WriteFile(t, filepath.Join(workDir, ".tool.download"), []byte(unpacktest.ElfExecutable))
	s := domainstep.NewPortableStep("portable", ".tool.download", "bin", "", true)
	s.Name = "tool"

	err := portable.NewHandler(unpacktest.TestMaxBytes).Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	assert.Equal(t, unpacktest.ElfExecutable, unpacktest.ReadString(t, filepath.Join(workDir, "bin", "tool")))
	assert.NoFileExists(t, from)
}

func TestHandler_Execute_UnsafeStepNameFails(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool"), []byte(unpacktest.ElfExecutable))
	s := domainstep.NewPortableStep("portable", "tool", "bin", "", true)
	s.Name = "../x"

	err := portable.NewHandler(unpacktest.TestMaxBytes).Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	assert.ErrorIs(t, err, portable.ErrInvalidName)
}

func TestHandler_Execute_MaxBytesLimitsExecutable(t *testing.T) {
	workDir := t.TempDir()
	unpacktest.WriteFile(t, filepath.Join(workDir, "tool"), []byte(unpacktest.ElfExecutable))
	s := domainstep.NewPortableStep("portable", "tool", "bin", "", true)

	err := portable.NewHandler(1).Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.ErrorIs(t, err, unpack.ErrTooLarge)
	assert.FileExists(t, filepath.Join(workDir, "tool"))
}
