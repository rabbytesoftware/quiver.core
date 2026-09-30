package host_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestNew_Defaults(t *testing.T) {
	h := host.New()

	assert.Equal(t, runtime.GOARCH, h.GOARCH)
	assert.NotNil(t, h.Commander)
	assert.Empty(t, h.HomeDir)
	assert.Nil(t, h.AppsDirs)
}

func TestHost_Layout_ExplicitDirs(t *testing.T) {
	f := mocks.NewSandbox(t, "linux")

	l, err := f.Host().Layout()

	require.NoError(t, err)
	assert.Equal(t, models.Layout{
		Bin:        f.Bin,
		Namespaces: f.NsDir,
		UserHome:   f.UserHome,
		AppData:    filepath.Join(f.UserHome, "AppData", "Roaming"),
		Apps:       f.Apps,
	}, l)
}

func TestHost_Layout_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	nsBlocked := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nsBlocked, "namespaces"), nil, 0o600))

	testCases := []struct {
		name string
		home string
	}{
		{
			name: "bin unavailable",
			home: file,
		},
		{
			name: "namespaces unavailable",
			home: nsBlocked,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := host.New()
			h.HomeDir = tc.home

			_, err := h.Layout()

			require.Error(t, err)
		})
	}
}

func TestHost_Layout_UserHomeUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.UserHomeDir reads USERPROFILE on windows")
	}
	t.Setenv("HOME", "")
	h := host.New()
	h.HomeDir = t.TempDir()

	_, err := h.Layout()

	require.Error(t, err)
}

func TestHost_Layout_DefaultHomeUsesProcessHome(t *testing.T) {
	quiverHome := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("QUIVER_HOME", quiverHome)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	h := host.New()
	h.Env = func(string) string { return "" }

	l, err := h.Layout()
	require.NoError(t, err)

	wantBin, err := paths.BinAt(quiverHome)
	require.NoError(t, err)
	wantNamespaces, err := paths.NamespacesAt(quiverHome)
	require.NoError(t, err)
	assert.Equal(t, wantBin, l.Bin)
	assert.Equal(t, wantNamespaces, l.Namespaces)
	assert.Equal(t, userHome, l.UserHome)
	assert.Equal(t, []string{"/Applications", filepath.Join(userHome, "Applications")}, l.Apps)
}

func TestHost_Layout_AppData(t *testing.T) {
	f := mocks.NewSandbox(t, "windows")

	l, err := f.Host().Layout()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(f.UserHome, "AppData", "Roaming"), l.AppData)

	f.Env["APPDATA"] = `C:\Roaming`
	l, err = f.Host().Layout()
	require.NoError(t, err)
	assert.Equal(t, `C:\Roaming`, l.AppData)
}

func TestHost_Layout_SandboxAppDataWinsOverEnv(t *testing.T) {
	h := host.New()
	h.HomeDir = t.TempDir()
	h.UserHomeDir = "/u"
	h.SandboxHome = "/h"
	h.Env = func(string) string { return "real" }

	l, err := h.Layout()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/h", "AppData", "Roaming"), l.AppData)
}
