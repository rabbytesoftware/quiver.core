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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestHandler_Execute_AppImage(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteAppImage(t, workDir, "bruno.AppImage", mocks.AppImageEntries("bruno"))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "bruno.AppImage", ".", "")

	require.NoError(t, err)
	appDir := filepath.Join(workDir, "bruno")
	assert.Equal(t, "#!/bin/sh\n", mocks.ReadString(t, filepath.Join(appDir, "AppRun")))
	assert.Contains(t, mocks.ReadString(t, filepath.Join(appDir, unpack.LauncherName)), "'--no-sandbox'")
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
	mocks.WriteFile(t, filepath.Join(workDir, "dl", "tool"), []byte(mocks.ElfExecutable))

	h := portable.NewHandler(mocks.TestMaxBytes)
	s := domainstep.NewPortableStep("portable", "missing", "${DIR}", "", true)
	s.From.OSArch = map[string]string{"linux/amd64": "dl/tool"}
	req := wizstep.Request{WorkDir: workDir, OSArch: "linux/amd64", Vars: map[string]string{"DIR": "bin"}}
	err := h.Execute(context.Background(), req, s)

	require.NoError(t, err)
	assert.Equal(t, mocks.ElfExecutable, mocks.ReadString(t, filepath.Join(workDir, "bin", "tool")))
}

func TestHandler_Execute_InvalidTimeout(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "tool"), []byte(mocks.ElfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, "tool", "bin", "soon")

	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(workDir, "bin"))
}

func TestHandler_Execute_SourceOutsideWorkdirKept(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(t.TempDir(), "tool"), []byte(mocks.ElfExecutable))

	err := runPortable(t, wizstep.Request{WorkDir: workDir}, from, "bin", "")

	require.NoError(t, err)
	assert.FileExists(t, from)
	assert.Equal(t, mocks.ElfExecutable, mocks.ReadString(t, filepath.Join(workDir, "bin", "tool")))
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
			name: "expired timeout",
			setup: func(t *testing.T, workDir string) string {
				mocks.WriteFile(t, filepath.Join(workDir, "a.tar"), mocks.HelloTar(t))
				return "a.tar"
			},
			to:      "out",
			timeout: "1ns",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "record path is a directory",
			setup: func(t *testing.T, workDir string) string {
				require.NoError(t, os.MkdirAll(filepath.Join(workDir, domain.PortableRecordFile, "x"), 0o755))
				mocks.WriteFile(t, filepath.Join(workDir, "Foo.zip"), mocks.ZipFiles(t, map[string]string{"Foo.app/Contents/MacOS/foo": "bin"}))
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

func TestHandler_Execute_ExecutableTakesTheStepName(t *testing.T) {
	workDir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(workDir, ".tool.download"), []byte(mocks.ElfExecutable))
	s := domainstep.NewPortableStep("portable", ".tool.download", "bin", "", true)
	s.Name = "tool"

	err := portable.NewHandler(mocks.TestMaxBytes).Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	assert.Equal(t, mocks.ElfExecutable, mocks.ReadString(t, filepath.Join(workDir, "bin", "tool")))
	assert.NoFileExists(t, from)
}

func TestHandler_Execute_MaxBytesLimitsExecutable(t *testing.T) {
	workDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(workDir, "tool"), []byte(mocks.ElfExecutable))
	s := domainstep.NewPortableStep("portable", "tool", "bin", "", true)

	err := portable.NewHandler(1).Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.ErrorIs(t, err, unpack.ErrTooLarge)
	assert.FileExists(t, filepath.Join(workDir, "tool"))
}
