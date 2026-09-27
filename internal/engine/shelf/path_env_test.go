package shelf

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShelf_RCFiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	testCases := []struct {
		name  string
		goos  string
		shell string
		want  []string
	}{
		{name: "zsh", goos: "linux", shell: "/bin/zsh", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "bash linux", goos: "linux", shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc")}},
		{name: "bash darwin", goos: goosDarwin, shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".bash_profile")}},
		{name: "fish", goos: "linux", shell: "/usr/local/bin/fish", want: []string{filepath.Join(home, ".config", "fish", "conf.d", "quiver.fish")}},
		{name: "unset", goos: "linux", shell: "", want: []string{filepath.Join(home, ".profile")}},
		{name: "unset on darwin", goos: goosDarwin, shell: "", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "sh", goos: "linux", shell: "/bin/sh", want: []string{filepath.Join(home, ".profile")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.goos)
			f.env["SHELL"] = tc.shell
			assert.Equal(t, tc.want, f.shelf.rcFiles(home))
		})
	}
}

func TestRCBlock(t *testing.T) {
	assert.Equal(t, "# quiver path\nexport PATH=\"$PATH:/q/bin\"\n", rcBlock("/h/.zshrc", "/q/bin"))
	assert.Equal(t, "# quiver path\ncontains -- \"/q/bin\" $PATH; or set -gx PATH $PATH \"/q/bin\"\n", rcBlock("/h/quiver.fish", "/q/bin"))
}

func TestShelf_PathStatus_OnPath(t *testing.T) {
	goos := "linux"
	if runtime.GOOS == goosWindows {
		goos = goosWindows
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
			f.env["PATH"] = tc.path(f.bin)

			got, err := f.shelf.pathStatus()

			require.NoError(t, err)
			assert.Equal(t, tc.want, got.OnPath)
			assert.Equal(t, f.bin, got.BinDir)
		})
	}
}

func TestShelf_SetupPath_WritesOnce(t *testing.T) {
	f := newFixture(t, goosDarwin)
	f.env["SHELL"] = "/bin/bash"
	bashrc := filepath.Join(f.userHome, ".bashrc")
	profile := filepath.Join(f.userHome, ".bash_profile")
	writeFile(t, bashrc, "alias x=y", 0o600)

	before, err := f.shelf.pathStatus()
	require.NoError(t, err)
	_, err = f.shelf.setupPath()
	require.NoError(t, err)
	after, err := f.shelf.setupPath()
	require.NoError(t, err)

	assert.False(t, before.Configured)
	assert.True(t, after.Configured)
	assert.Equal(t, []string{bashrc, profile}, after.Files)
	block := rcBlock(bashrc, f.bin)
	data, err := os.ReadFile(bashrc)
	require.NoError(t, err)
	assert.Equal(t, "alias x=y\n\n"+block, string(data))
	data, err = os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, block, string(data))
}

func TestShelf_SetupPath_SeparatesFromTrailingNewline(t *testing.T) {
	f := newFixture(t, "linux")
	f.env["SHELL"] = "/bin/zsh"
	zshrc := filepath.Join(f.userHome, ".zshrc")
	writeFile(t, zshrc, "alias x=y\n", 0o600)

	_, err := f.shelf.setupPath()
	require.NoError(t, err)

	data, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.Equal(t, "alias x=y\n\n"+rcBlock(zshrc, f.bin), string(data))
}

func TestShelf_SetupPath_Fish(t *testing.T) {
	f := newFixture(t, "linux")
	f.env["SHELL"] = "/usr/bin/fish"

	got, err := f.shelf.setupPath()
	require.NoError(t, err)

	assert.True(t, got.Configured)
	data, err := os.ReadFile(got.Files[0])
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), pathMarker))
	assert.Contains(t, string(data), "set -gx PATH")
}

func TestShelf_PathEnv_Errors(t *testing.T) {
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
				f.shelf.homeDir = file
			},
			wantStatus: true,
		},
		{
			name: "rc file unreadable",
			setup: func(t *testing.T, f *fixture) {
				require.NoError(t, os.MkdirAll(filepath.Join(f.userHome, ".zshrc"), 0o750))
			},
			wantStatus: true,
		},
		{
			name: "rc dir not creatable",
			setup: func(t *testing.T, f *fixture) {
				f.env["SHELL"] = "fish"
				f.shelf.userHomeDir = readOnlyDir(t)
			},
		},
		{
			name: "rc file not writable",
			setup: func(t *testing.T, f *fixture) {
				f.shelf.userHomeDir = readOnlyDir(t)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "linux")
			f.env["SHELL"] = "zsh"
			tc.setup(t, f)

			_, statusErr := f.shelf.pathStatus()
			_, setupErr := f.shelf.setupPath()

			assert.Equal(t, tc.wantStatus, statusErr != nil)
			assert.Error(t, setupErr)
		})
	}
}

func TestShelf_PathEnv_Windows(t *testing.T) {
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
			f := newFixture(t, goosWindows)
			f.userPath.value = tc.current(f.bin)

			before, err := f.shelf.pathStatus()
			require.NoError(t, err)
			after, err := f.shelf.setupPath()
			require.NoError(t, err)
			_, err = f.shelf.setupPath()
			require.NoError(t, err)

			assert.Equal(t, tc.wantConfigured, before.Configured)
			assert.True(t, after.Configured)
			assert.Equal(t, []string{userPathLocation}, after.Files)
			assert.Equal(t, tc.wantValue(f.bin), f.userPath.value)
			assert.Equal(t, tc.wantWrites, f.userPath.writes)
		})
	}
}

func TestShelf_PathEnv_WindowsErrors(t *testing.T) {
	boom := errors.New("boom")

	f := newFixture(t, goosWindows)
	f.userPath.readErr = boom
	_, err := f.shelf.pathStatus()
	assert.ErrorIs(t, err, boom)
	_, err = f.shelf.setupPath()
	assert.ErrorIs(t, err, boom)

	f = newFixture(t, goosWindows)
	f.userPath.writeErr = boom
	_, err = f.shelf.setupPath()
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

func TestShelf_PathMethods_UseContextFreeHelpers(t *testing.T) {
	f := newFixture(t, goosWindows)

	_, err := f.shelf.PathStatus(context.Background())
	require.NoError(t, err)
}
