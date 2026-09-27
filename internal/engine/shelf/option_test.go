package shelf

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptions_SetFields(t *testing.T) {
	cmd := &stubCommander{}
	tagger := &stubTagger{}
	up := &stubUserPath{}

	s := New(
		WithHomeDir("/q"),
		WithUserHomeDir("/u"),
		WithAppsDirs([]string{"/a", "/b"}),
		WithGOOS("plan9"),
		WithGOARCH("mips"),
		WithCommander(cmd),
		WithEnv(func(string) string { return "v" }),
		withTagger(tagger),
		withUserPath(up),
	).(*shelf)

	assert.Equal(t, "/q", s.homeDir)
	assert.Equal(t, "/u", s.userHomeDir)
	assert.Equal(t, []string{"/a", "/b"}, s.appsDirs)
	assert.Equal(t, "plan9", s.goos)
	assert.Equal(t, "mips", s.goarch)
	assert.Same(t, cmd, s.commander)
	assert.Equal(t, "v", s.env("ANY"))
	assert.Same(t, tagger, s.tagger)
	assert.Same(t, up, s.userPath)
}

func TestWithSandboxHome_ResolvesNothingOutsideHome(t *testing.T) {
	testCases := []struct {
		name string
		goos string
	}{
		{name: "darwin", goos: goosDarwin},
		{name: "linux", goos: "linux"},
		{name: "windows", goos: goosWindows},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			s := New(
				WithGOOS(tc.goos),
				WithEnv(func(key string) string { return "/real/" + key }),
				WithSandboxHome(home),
			).(*shelf)

			l, err := s.layout()
			require.NoError(t, err)

			dirs := append([]string{
				l.bin,
				l.namespaces,
				l.userHome,
				xdgDir(l.userHome),
				lnkDir(s.appData(l.userHome)),
			}, l.apps...)
			for _, dir := range dirs {
				rel, err := filepath.Rel(home, dir)
				require.NoError(t, err)
				assert.False(t, strings.HasPrefix(rel, ".."), "%s escapes %s", dir, home)
			}
			assert.Equal(t, []string{filepath.Join(home, "Applications")}, l.apps)
		})
	}
}

func TestWithSandboxHome_AppDataIgnoresOptionOrder(t *testing.T) {
	realEnv := WithEnv(func(key string) string { return "real-" + key })
	want := filepath.Join("/h", "AppData", "Roaming")
	testCases := []struct {
		name string
		opts []Option
	}{
		{name: "env before sandbox", opts: []Option{realEnv, WithSandboxHome("/h")}},
		{name: "env after sandbox", opts: []Option{WithSandboxHome("/h"), realEnv}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.opts...).(*shelf)

			assert.Equal(t, want, s.appData("/u"))
			assert.Equal(t, "real-PATH", s.env("PATH"))
			assert.Equal(t, "real-SHELL", s.env("SHELL"))
		})
	}
}
