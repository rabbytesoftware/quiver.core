package pathenv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

type rcFixture struct {
	*mocks.Sandbox
	rc *rc
}

func newLinuxRC(
	t *testing.T,
) *rcFixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "linux")
	return &rcFixture{Sandbox: sb, rc: NewRC(sb.Host(), "", []string{".bashrc"}).(*rc)}
}

func newDarwinRC(
	t *testing.T,
) *rcFixture {
	t.Helper()
	sb := mocks.NewSandbox(t, "darwin")
	return &rcFixture{Sandbox: sb, rc: NewRC(sb.Host(), "zsh", []string{".bashrc", ".bash_profile"}).(*rc)}
}

func TestRC_Files(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")

	testCases := []struct {
		name   string
		darwin bool
		shell  string
		want   []string
	}{
		{name: "zsh", shell: "/bin/zsh", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "bash linux", shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc")}},
		{name: "bash darwin", darwin: true, shell: "/bin/bash", want: []string{filepath.Join(home, ".bashrc"), filepath.Join(home, ".bash_profile")}},
		{name: "fish", shell: "/usr/local/bin/fish", want: []string{filepath.Join(home, ".config", "fish", "conf.d", "quiver.fish")}},
		{name: "unset", shell: "", want: []string{filepath.Join(home, ".profile")}},
		{name: "unset on darwin", darwin: true, shell: "", want: []string{filepath.Join(home, ".zshrc")}},
		{name: "sh", shell: "/bin/sh", want: []string{filepath.Join(home, ".profile")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLinuxRC(t)
			if tc.darwin {
				f = newDarwinRC(t)
			}
			f.Env["SHELL"] = tc.shell
			assert.Equal(t, tc.want, f.rc.files(home))
		})
	}
}

func TestRCBlock(t *testing.T) {
	assert.Equal(t, "# quiver path\nexport PATH=\"$PATH:/q/bin\"\n", rcBlock("/h/.zshrc", "/q/bin"))
	assert.Equal(t, "# quiver path\ncontains -- \"/q/bin\" $PATH; or set -gx PATH $PATH \"/q/bin\"\n", rcBlock("/h/quiver.fish", "/q/bin"))
}

func TestRC_Status_OnPath(t *testing.T) {
	testCases := []struct {
		name string
		path func(bin string) string
		want bool
	}{
		{name: "absent", path: func(string) string { return "/usr/bin:/bin" }, want: false},
		{name: "present", path: func(bin string) string { return "/usr/bin:" + bin + string(filepath.Separator) }, want: true},
		{name: "empty", path: func(string) string { return "" }, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mocks.RequireUnixHost(t)
			f := newLinuxRC(t)
			f.Env["PATH"] = tc.path(f.Bin)

			got, err := f.rc.Status(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tc.want, got.OnPath)
			assert.Equal(t, f.Bin, got.BinDir)
		})
	}
}

func TestRC_Setup_WritesOnce(t *testing.T) {
	f := newDarwinRC(t)
	f.Env["SHELL"] = "/bin/bash"
	bashrc := filepath.Join(f.UserHome, ".bashrc")
	profile := filepath.Join(f.UserHome, ".bash_profile")
	mocks.WriteFile(t, bashrc, "alias x=y", 0o600)

	before, err := f.rc.Status(context.Background())
	require.NoError(t, err)
	_, err = f.rc.Setup(context.Background())
	require.NoError(t, err)
	after, err := f.rc.Setup(context.Background())
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

func TestRC_Setup_SeparatesFromTrailingNewline(t *testing.T) {
	f := newLinuxRC(t)
	f.Env["SHELL"] = "/bin/zsh"
	zshrc := filepath.Join(f.UserHome, ".zshrc")
	mocks.WriteFile(t, zshrc, "alias x=y\n", 0o600)

	_, err := f.rc.Setup(context.Background())
	require.NoError(t, err)

	data, err := os.ReadFile(zshrc)
	require.NoError(t, err)
	assert.Equal(t, "alias x=y\n\n"+rcBlock(zshrc, f.Bin), string(data))
}

func TestRC_Setup_Fish(t *testing.T) {
	f := newLinuxRC(t)
	f.Env["SHELL"] = "/usr/bin/fish"

	got, err := f.rc.Setup(context.Background())
	require.NoError(t, err)

	assert.True(t, got.Configured)
	data, err := os.ReadFile(got.Files[0])
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), pathMarker))
	assert.Contains(t, string(data), "set -gx PATH")
}

func TestRC_Errors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	testCases := []struct {
		name       string
		setup      func(t *testing.T, f *rcFixture)
		wantStatus bool
	}{
		{
			name: "layout",
			setup: func(t *testing.T, f *rcFixture) {
				f.rc.host.HomeDir = file
			},
			wantStatus: true,
		},
		{
			name: "rc file unreadable",
			setup: func(t *testing.T, f *rcFixture) {
				require.NoError(t, os.MkdirAll(filepath.Join(f.UserHome, ".zshrc"), 0o750))
			},
			wantStatus: true,
		},
		{
			name: "rc dir not creatable",
			setup: func(t *testing.T, f *rcFixture) {
				f.Env["SHELL"] = "fish"
				f.rc.host.UserHomeDir = mocks.ReadOnlyDir(t)
			},
		},
		{
			name: "rc file not writable",
			setup: func(t *testing.T, f *rcFixture) {
				f.rc.host.UserHomeDir = mocks.ReadOnlyDir(t)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLinuxRC(t)
			f.Env["SHELL"] = "zsh"
			tc.setup(t, f)

			_, statusErr := f.rc.Status(context.Background())
			_, setupErr := f.rc.Setup(context.Background())

			assert.Equal(t, tc.wantStatus, statusErr != nil)
			assert.Error(t, setupErr)
		})
	}
}
