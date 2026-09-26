package transports_test

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
)

// tempSocketPath returns a short unique socket path safe on macOS (UNIX_PATH_MAX = 104).
func tempSocketPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "qv-*.sock")
	require.NoError(t, err)
	path := f.Name()
	f.Close()
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestSocketTransport_Listen_Fresh(t *testing.T) {
	path := tempSocketPath(t)

	transport := transports.NewSocket(path)

	ln, err := transport.Listen()
	require.NoError(t, err)
	defer ln.Close()

	_, err = os.Stat(path)
	assert.NoError(t, err, "socket file should exist after Listen")
}

func TestSocketTransport_Listen_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits not enforced on Windows")
	}
	path := tempSocketPath(t)

	transport := transports.NewSocket(path)

	ln, err := transport.Listen()
	require.NoError(t, err)
	defer ln.Close()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSocketTransport_Listen_StaleSocket(t *testing.T) {
	path := tempSocketPath(t)

	// Simulate a crash: create the socket but prevent auto-removal on close.
	// Go's net.Listen removes the file on Close() by default; SetUnlinkOnClose(false)
	// leaves the file behind, reproducing what happens after a daemon crash.
	stale, err := net.Listen("unix", path)
	require.NoError(t, err)
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	stale.Close()

	transport := transports.NewSocket(path)

	ln, err := transport.Listen()
	require.NoError(t, err)
	defer ln.Close()

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	conn.Close()
}

// TestSocketTransport_Listen_CreatesMissingParentDirectory is the regression
// test for a genuine first boot: PrepareGateway (internal/internal.go) now
// binds this before internal.New has run at all, which used to be what
// created the Quiver home directory itself (via core.New's log file) before
// this ever tried to bind inside it. On a machine with no ~/.quiver yet,
// nothing else creates that directory ahead of this call any more.
func TestSocketTransport_Listen_CreatesMissingParentDirectory(t *testing.T) {
	dir := tempSocketPath(t) + "-missing-parent"
	path := filepath.Join(dir, "quiver.sock")
	t.Cleanup(func() { os.RemoveAll(dir) })

	_, err := os.Stat(dir)
	require.True(t, os.IsNotExist(err), "the parent directory must not exist yet -- that is the condition under test")

	transport := transports.NewSocket(path)

	ln, err := transport.Listen()
	require.NoError(t, err, "Listen must create its own missing parent directory rather than assuming something else already has")
	defer ln.Close()

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	conn.Close()
}

// A merely-missing parent directory no longer proves a Listen error: Listen
// now creates it (see TestSocketTransport_Listen_CreatesMissingParentDirectory),
// and CI's Docker runner has permission to create one anywhere, unlike a
// non-root workstation where "/nonexistent" happened to fail for a different
// reason. A path through a regular file has no directory entries under it,
// so MkdirAll fails there regardless of the caller's privilege level.
func TestSocketTransport_Listen_ListenError(t *testing.T) {
	f, err := os.CreateTemp("", "qv-listen-error-*")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() { os.Remove(f.Name()) })

	transport := transports.NewSocket(filepath.Join(f.Name(), "subdir", "quiver.sock"))

	_, err = transport.Listen()
	assert.Error(t, err)
}

func TestSocketTransport_Listen_DaemonAlreadyRunning(t *testing.T) {
	path := tempSocketPath(t)

	active, err := net.Listen("unix", path)
	require.NoError(t, err)
	defer active.Close()

	go func() {
		conn, err := active.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()

	transport := transports.NewSocket(path)

	_, err = transport.Listen()
	assert.ErrorIs(t, err, transports.ErrDaemonRunning)
}

func TestSocketTransport_Listen_ChmodError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod is not applied on Windows")
	}
	path := tempSocketPath(t)
	restore := transports.SetChmod(func(string, os.FileMode) error {
		return os.ErrPermission
	})
	defer restore()

	transport := transports.NewSocket(path)

	_, err := transport.Listen()
	assert.ErrorIs(t, err, os.ErrPermission)

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "socket file should be cleaned up after chmod failure")
}

func TestSocketListener_Close_RemovesFile(t *testing.T) {
	path := tempSocketPath(t)

	transport := transports.NewSocket(path)

	ln, err := transport.Listen()
	require.NoError(t, err)

	require.NoError(t, ln.Close())

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "socket file should be removed after Close")
}
