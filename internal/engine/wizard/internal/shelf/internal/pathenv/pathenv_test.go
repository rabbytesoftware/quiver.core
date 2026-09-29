package pathenv

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

type fixture struct {
	*mocks.Sandbox
	manager *manager
}

func newFixture(
	t *testing.T,
	goos string,
) *fixture {
	t.Helper()
	sb := mocks.NewSandbox(t, goos)
	return &fixture{
		Sandbox: sb,
		manager: New(sb.Host(), sb.UserPath).(*manager),
	}
}

func TestManager_RCFiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	testCases := []struct {
		name  string
		goos  string
		shell string
		want  []string
	}{
		{name: "zsh", goos: "linux", shell: "/bin/zsh", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "bash linux", goos: "linux", shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc")}},
		{name: "bash darwin", goos: platform.GOOSDarwin, shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".bash_profile")}},
		{name: "fish", goos: "linux", shell: "/usr/local/bin/fish", want: []string{filepath.Join(home, ".config", "fish", "conf.d", "quiver.fish")}},
		{name: "unset", goos: "linux", shell: "", want: []string{filepath.Join(home, ".profile")}},
		{name: "unset on darwin", goos: platform.GOOSDarwin, shell: "", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "sh", goos: "linux", shell: "/bin/sh", want: []string{filepath.Join(home, ".profile")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			f.Env["SHELL"] = tc.shell
			assert.Equal(t, tc.want, f.manager.rcFiles(home))
		})
	}
}

func TestRCBlock(t *testing.T) {
	assert.Equal(t, "# quiver path\nexport PATH=\"$PATH:/q/bin\"\n", rcBlock("/h/.zshrc", "/q/bin"))
	assert.Equal(t, "# quiver path\ncontains -- \"/q/bin\" $PATH; or set -gx PATH $PATH \"/q/bin\"\n", rcBlock("/h/quiver.fish", "/q/bin"))
}

func TestManager_PathStatus_OnPath(t *testing.T) {
	goos := "linux"
	if runtime.GOOS == platform.GOOSWindows {
		goos = platform.GOOSWindows
	}
	sep := string(os.PathListSeparator)

	testCases := []struct {
		name string
		path func(bin string) string
		want bool
	}{
		{name: "absent", path: func(string) string { return "/usr/bin" + sep + "/bin" }, want: false},
		{name: "present", path: func(bin string) string { return "/usr/bin" + sep + bin + string(filepath.Separator) }, want: true},
		{name: "empty", path: func(string) string { return "" }, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, goos)
			f.Env["PATH"] = tc.path(f.Bin)

			got, err := f.manager.Status()

			require.NoError(t, err)
			assert.Equal(t, tc.want, got.OnPath)
			assert.Equal(t, f.Bin, got.BinDir)
		})
	}
}

func TestManager_SetupPath_WritesOnce(t *testing.T) {
	f := newFixture(t, platform.GOOSDarwin)
	f.Env["SHELL"] = "/bin/bash"
	bashrc := filepath.Join(f.UserHome, ".bashrc")
	profile := filepath.Join(f.UserHome, ".bash_profile")
	mocks.WriteFile(t, bashrc, "alias x=y", 0o600)

	before, err := f.manager.Status()
	require.NoError(t, err)
	_, err = f.manager.Setup()
	require.NoError(t, err)
	after, err := f.manager.Setup()
	require.NoError(t, err)

	assert.False(t, before.Configured)
	assert.True(t, after.Configured)
	assert.Equal(t, []string{bashrc, profile}, after.Files)
	block := rcBlock(bashrc, f.Bin)
	data, err := os.ReadFile(bashrc)
	require.NoError(t, err)
	assert.Equal(t, "alias x=y\n\n"+block, string(data))
	data, err = os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, block, string(data))
}

func TestManager_SetupPath_SeparatesFromTrailingNewline(t *testing.T) {
	f := newFixture(t, "linux")
	f.Env["SHELL"] = "/bin/zsh"
	zshrc := filepath.Join(f.UserHome, ".zshrc")
	mocks.WriteFile(t, zshrc, "alias x=y\n", 0o600)

	_, err := f.manager.Setup()
	require.NoError(t, err)

	data, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.Equal(t, "alias x=y\n\n"+rcBlock(zshrc, f.Bin), string(data))
}

func TestManager_SetupPath_Fish(t *testing.T) {
	f := newFixture(t, "linux")
	f.Env["SHELL"] = "/usr/bin/fish"

	got, err := f.manager.Setup()
	require.NoError(t, err)

	assert.True(t, got.Configured)
	data, err := os.ReadFile(got.Files[0])
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), pathMarker))
	assert.Contains(t, string(data), "set -gx PATH")
}

func TestManager_PathEnv_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name       string
		setup      func(t *testing.T, f *fixture)
		wantStatus bool
	}{
		{
			name: "layout",
			setup: func(t *testing.T, f *fixture) {
				f.manager.host.HomeDir = file
			},
			wantStatus: true,
		},
		{
			name: "rc file unreadable",
			setup: func(t *testing.T, f *fixture) {
				require.NoError(t, os.MkdirAll(filepath.Join(f.UserHome, ".zshrc"), 0o750))
			},
			wantStatus: true,
		},
		{
			name: "rc dir not creatable",
			setup: func(t *testing.T, f *fixture) {
				f.Env["SHELL"] = "fish"
				f.manager.host.UserHomeDir = mocks.ReadOnlyDir(t)
			},
		},
		{
			name: "rc file not writable",
			setup: func(t *testing.T, f *fixture) {
				f.manager.host.UserHomeDir = mocks.ReadOnlyDir(t)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "linux")
			f.Env["SHELL"] = "zsh"
			tc.setup(t, f)

			_, statusErr := f.manager.Status()
			_, setupErr := f.manager.Setup()

			assert.Equal(t, tc.wantStatus, statusErr != nil)
			assert.Error(t, setupErr)
		})
	}
}

func TestManager_PathEnv_Windows(t *testing.T) {
	testCases := []struct {
		name           string
		current        func(bin string) string
		wantConfigured bool
		wantValue      func(bin string) string
		wantWrites     int
	}{
		{
			name:       "empty",
			current:    func(string) string { return "" },
			wantValue:  func(bin string) string { return bin },
			wantWrites: 1,
		},
		{
			name:       "others with trailing separator",
			current:    func(string) string { return `C:\a;` },
			wantValue:  func(bin string) string { return `C:\a;` + bin },
			wantWrites: 1,
		},
		{
			name:           "already present in another case",
			current:        func(bin string) string { return `C:\a;` + strings.ToUpper(bin) },
			wantConfigured: true,
			wantValue:      func(bin string) string { return `C:\a;` + strings.ToUpper(bin) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, platform.GOOSWindows)
			f.UserPath.Value = tc.current(f.Bin)

			before, err := f.manager.Status()
			require.NoError(t, err)
			after, err := f.manager.Setup()
			require.NoError(t, err)
			_, err = f.manager.Setup()
			require.NoError(t, err)

			assert.Equal(t, tc.wantConfigured, before.Configured)
			assert.True(t, after.Configured)
			assert.Equal(t, []string{userPathLocation}, after.Files)
			assert.Equal(t, tc.wantValue(f.Bin), f.UserPath.Value)
			assert.Equal(t, tc.wantWrites, f.UserPath.Writes)
		})
	}
}

func TestManager_PathEnv_WindowsErrors(t *testing.T) {
	boom := errors.New("boom")

	f := newFixture(t, platform.GOOSWindows)
	f.UserPath.ReadErr = boom
	_, err := f.manager.Status()
	assert.ErrorIs(t, err, boom)
	_, err = f.manager.Setup()
	assert.ErrorIs(t, err, boom)

	f = newFixture(t, platform.GOOSWindows)
	f.UserPath.WriteErr = boom
	_, err = f.manager.Setup()
	assert.ErrorIs(t, err, boom)
}

func TestPathListContains(t *testing.T) {
	testCases := []struct {
		name string
		list string
		dir  string
		sep  string
		fold bool
		want bool
	}{
		{name: "exact", list: "/a:/b", dir: "/b", sep: ":", want: true},
		{name: "cleaned", list: "/a:/b/", dir: "/b", sep: ":", want: true},
		{name: "case sensitive", list: "/a:/B", dir: "/b", sep: ":", want: false},
		{name: "case folded", list: `C:\A;C:\B`, dir: `c:\b`, sep: ";", fold: true, want: true},
		{name: "empty entries", list: "::", dir: "/b", sep: ":", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, pathListContains(tc.list, tc.dir, tc.sep, tc.fold))
		})
	}
}
