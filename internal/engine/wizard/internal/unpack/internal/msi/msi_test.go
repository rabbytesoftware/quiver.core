package msi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

var errRunner = errors.New("runner failed")

func writePackage(
	t *testing.T,
	name string,
	data []byte,
) string {
	t.Helper()

	return mocks.WriteFile(t, filepath.Join(t.TempDir(), name), data)
}

func oleFile() []byte {
	return append([]byte(oleMagic), make([]byte, 504)...)
}

func detectWith(
	t *testing.T,
	path string,
	run runner,
) (models.Format, bool) {
	t.Helper()

	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	format, ok, err := newWith(mocks.TestMaxBytes, guard.HostRules{FoldCase: true, WindowsNames: true}, run)(src, info.Size())
	require.NoError(t, err)

	return format, ok
}

// adminImage fakes msiexec /a: it lays files out under TARGETDIR the way an
// administrative install mirrors the Directory table.
func adminImage(
	t *testing.T,
	files map[string]string,
	code int,
	seen *invocation,
) runner {
	t.Helper()

	return func(_ context.Context, inv invocation) (int, error) {
		*seen = inv
		for name, body := range files {
			mocks.WriteFile(t, filepath.Join(inv.target, filepath.FromSlash(name)), []byte(body))
		}
		mocks.WriteFile(t, inv.log, []byte("=== Logging started ===\nAction ended: return value 3.\n"))
		return code, nil
	}
}

func TestNew_Detection(t *testing.T) {
	testCases := []struct {
		name string
		file string
		data []byte
		want bool
	}{
		{name: "compound file with msi extension", file: "setup.msi", data: oleFile(), want: true},
		{name: "upper case extension", file: "SETUP.MSI", data: oleFile(), want: true},
		{name: "fletcher download name", file: ".tool.download.msi", data: oleFile(), want: true},
		{name: "compound file without msi extension", file: "setup.doc", data: oleFile()},
		{name: "msi extension without compound file magic", file: "setup.msi", data: []byte("PK\x03\x04 zip")},
		{name: "too short", file: "setup.msi", data: []byte{0xd0, 0xcf}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			format, ok := detectWith(t, writePackage(t, tc.file, tc.data), nil)

			assert.Equal(t, tc.want, ok)
			if !tc.want {
				return
			}
			assert.Equal(t, models.KindMsi, format.Kind())
			assert.Equal(t, models.Unit{}, format.Unit())
		})
	}
}

func TestUnpack_HoistsTheApplicationDirectory(t *testing.T) {
	for _, code := range []int{exitSuccess, exitRebootRequired} {
		var seen invocation
		from := writePackage(t, "Setup.msi", oleFile())
		run := adminImage(t, map[string]string{
			"Setup.msi":                        "copy",
			"PFiles/Vendor/App/app.exe":        "app",
			"PFiles/Vendor/App/Helper.EXE":     "helper",
			"PFiles/Vendor/App/lib/x.dll":      "dll",
			"PFiles/Vendor/App/docs/guide.exe": "nested",
			"System64/common.dll":              "common",
		}, code, &seen)
		format, ok := detectWith(t, from, run)
		require.True(t, ok)
		to := filepath.Join(t.TempDir(), "tool")

		result, err := format.Unpack(context.Background(), models.Target{Dir: to})

		require.NoError(t, err)
		assert.Equal(t, []models.App{
			{Name: "Helper", Entry: filepath.Join(to, "Helper.EXE")},
			{Name: "app", Entry: filepath.Join(to, "app.exe")},
		}, result.Apps)
		assert.Equal(t, "dll", mocks.ReadString(t, filepath.Join(to, "lib", "x.dll")))
		assert.Equal(t, "common", mocks.ReadString(t, filepath.Join(to, "System64", "common.dll")))
		assert.NoDirExists(t, filepath.Join(to, "PFiles"))
		assert.NoFileExists(t, filepath.Join(to, "Setup.msi"))
		assert.True(t, filepath.IsAbs(seen.msi))
		assert.Equal(t, filepath.Dir(seen.target), filepath.Dir(seen.log))
		assert.NoDirExists(t, filepath.Dir(seen.target))
	}
}

func TestUnpack_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		run     runner
		ctx     func() context.Context
		to      func(t *testing.T) string
		wantErr error
	}{
		{
			name:    "msiexec fails",
			run:     adminImage(t, nil, 1603, &invocation{}),
			wantErr: models.ErrMsiexecFailed,
		},
		{
			name:    "msiexec cannot start",
			run:     func(context.Context, invocation) (int, error) { return 0, errRunner },
			wantErr: errRunner,
		},
		{
			name: "canceled while msiexec runs",
			run:  adminImage(t, nil, 1602, &invocation{}),
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: context.Canceled,
		},
		{
			name:    "msiexec leaves no image",
			run:     func(context.Context, invocation) (int, error) { return exitSuccess, nil },
			wantErr: os.ErrNotExist,
		},
		{
			name: "image holds an escaping link",
			run: func(_ context.Context, inv invocation) (int, error) {
				app := filepath.Join(inv.target, "PFiles", "App")
				mocks.WriteFile(t, filepath.Join(app, "app.exe"), []byte("app"))
				return exitSuccess, os.Symlink(filepath.Join("..", "..", "etc"), filepath.Join(app, "evil"))
			},
			wantErr: models.ErrEscape,
		},
		{
			name: "destination under a file",
			run:  adminImage(t, nil, exitSuccess, &invocation{}),
			to: func(t *testing.T) string {
				return filepath.Join(mocks.WriteFile(t, filepath.Join(t.TempDir(), "file"), []byte("x")), "tool")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			format, ok := detectWith(t, writePackage(t, "setup.msi", oleFile()), tc.run)
			require.True(t, ok)
			ctx := context.Background()
			if tc.ctx != nil {
				ctx = tc.ctx()
			}
			to := filepath.Join(t.TempDir(), "tool")
			if tc.to != nil {
				to = tc.to(t)
			}

			_, err := format.Unpack(ctx, models.Target{Dir: to})

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}
