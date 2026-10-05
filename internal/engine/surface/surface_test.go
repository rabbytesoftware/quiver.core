package surface_test

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

// shortDir returns a directory with a path short enough for a unix socket
// (t.TempDir's per-test name can exceed the limit).
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "qs")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestSocketPath_IsDeterministicAndShort(t *testing.T) {
	e := surface.New("/home/u/.quiver/run")
	ns := domain.Namespace("github.com/user/repo@v1")

	a, b := e.SocketPath(ns), e.SocketPath(ns)
	require.Equal(t, a, b)
	require.NotEqual(t, a, e.SocketPath("github.com/user/other"))
	require.Equal(t, filepath.FromSlash("/home/u/.quiver/run"), filepath.Dir(a))
	require.Regexp(t, `^[0-9a-f]{12}\.sock$`, filepath.Base(a))
}

func TestPrepare_CreatesPrivateDirAndReturnsPath(t *testing.T) {
	run := filepath.Join(shortDir(t), "run")
	e := surface.New(run)

	path, err := e.Prepare("github.com/user/repo")
	require.NoError(t, err)
	require.Equal(t, e.SocketPath("github.com/user/repo"), path)

	info, err := os.Stat(run)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestPrepare_RemovesStaleSocket(t *testing.T) {
	e := surface.New(shortDir(t))
	path := e.SocketPath("a/b")
	require.NoError(t, os.WriteFile(path, nil, 0o600)) // a dead file, nothing listening

	_, err := e.Prepare("a/b")
	require.NoError(t, err)
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
}

func TestPrepare_KeepsLiveSocket(t *testing.T) {
	e := surface.New(shortDir(t))
	path := e.SocketPath("a/b")
	l, err := net.Listen("unix", path)
	require.NoError(t, err)
	defer l.Close()

	got, err := e.Prepare("a/b")
	require.NoError(t, err)
	require.Equal(t, path, got)
	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "a live socket must survive Prepare")
}

func TestPrepare_PathTooLong(t *testing.T) {
	e := surface.New("/" + strings.Repeat("a", 120))

	_, err := e.Prepare("a/b")
	require.ErrorIs(t, err, surface.ErrPathTooLong)
}

func TestCleanup_RemovesSocket(t *testing.T) {
	e := surface.New(shortDir(t))
	path, err := e.Prepare("a/b")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	e.Cleanup("a/b")
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
}

func TestHandlerAndReady_NoSurfaceMode(t *testing.T) {
	e := surface.New(shortDir(t))

	_, err := e.Handler(surface.Spec{})
	require.ErrorIs(t, err, surface.ErrNoSurface)
	require.False(t, e.Ready(t.Context(), surface.Spec{}, "/"))
}

func TestPrepare_RunDirUnderAFileFails(t *testing.T) {
	file := filepath.Join(shortDir(t), "f")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	_, err := surface.New(filepath.Join(file, "run")).Prepare("a/b")
	require.Error(t, err)
}

func TestPrepare_UnremovableStalePathFails(t *testing.T) {
	e := surface.New(shortDir(t))
	path := e.SocketPath("a/b")
	require.NoError(t, os.MkdirAll(filepath.Join(path, "child"), 0o700))

	_, err := e.Prepare("a/b")
	require.Error(t, err)
}
