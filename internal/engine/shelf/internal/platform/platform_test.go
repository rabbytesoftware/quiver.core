package platform_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

func TestHost_Layout_ExplicitDirs(t *testing.T) {
	f := mocks.NewSandbox(t, "linux")

	l, err := f.Host().Layout()

	require.NoError(t, err)
	assert.Equal(t, platform.Layout{Bin: f.Bin, Namespaces: f.NsDir, UserHome: f.UserHome, Apps: f.Apps}, l)
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
			h := platform.NewHost()
			h.HomeDir = tc.home

			_, err := h.Layout()

			require.Error(t, err)
		})
	}
}

func TestHost_Layout_UserHomeUnavailable(t *testing.T) {
	if runtime.GOOS == platform.GOOSWindows {
		t.Skip("os.UserHomeDir reads USERPROFILE on windows")
	}
	t.Setenv("HOME", "")
	h := platform.NewHost()
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
	h := platform.NewHost()
	h.GOOS = "linux"
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

func TestNewHost_Defaults(t *testing.T) {
	h := platform.NewHost()

	assert.Equal(t, runtime.GOOS, h.GOOS)
	assert.Equal(t, runtime.GOARCH, h.GOARCH)
	assert.NotNil(t, h.Commander)
	assert.NotNil(t, h.Env)
	assert.NotNil(t, h.Rename)
	assert.NotNil(t, h.Chmod)
	assert.Empty(t, h.HomeDir)
	assert.Empty(t, h.UserHomeDir)
	assert.Nil(t, h.AppsDirs)
}

func TestHost_AppData(t *testing.T) {
	f := mocks.NewSandbox(t, platform.GOOSWindows)

	assert.Equal(t, filepath.Join("/home", "AppData", "Roaming"), f.Host().AppData("/home"))
	f.Env["APPDATA"] = `C:\Roaming`
	assert.Equal(t, `C:\Roaming`, f.Host().AppData("/home"))
}

func TestHost_AppData_SandboxWinsOverEnv(t *testing.T) {
	h := platform.NewHost()
	h.SandboxHome = "/h"
	h.Env = func(string) string { return "real" }

	assert.Equal(t, filepath.Join("/h", "AppData", "Roaming"), h.AppData("/u"))
}
