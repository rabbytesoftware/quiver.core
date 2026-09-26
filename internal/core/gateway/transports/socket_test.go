package transports_test

import (
	"net"
	"os"
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

func TestSocketTransport_Listen_ListenError(t *testing.T) {
	transport := transports.NewSocket("/nonexistent/dir/quiver.sock")

	_, err := transport.Listen()
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

// The production bug, as a regression test: a daemon that loses a race for
// its own socket path must not be able to delete the winner's live socket on
// its way out. This reproduces the race directly rather than the timing that
// causes it -- two listeners never need to race in real time for the second
// one's Close to run after a third party has already replaced the file at
// this path, which is why the fix checks file identity rather than trying to
// prevent the race itself.
func TestSocketListener_Close_LeavesAnotherListenersSocketAlone(t *testing.T) {
	path := tempSocketPath(t)

	transport := transports.NewSocket(path)

	loser, err := transport.Listen()
	require.NoError(t, err, "the loser must bind successfully to reproduce the race: only its Close, afterwards, is the hazard")

	// Stand-in for a second daemon that won the same path: removes the
	// loser's file and binds its own fresh listener there, exactly what
	// handleStale + net.Listen do together for a real second process.
	require.NoError(t, os.Remove(path))
	winner, err := net.Listen("unix", path)
	require.NoError(t, err)
	defer winner.Close()

	require.NoError(t, loser.Close(), "the loser's own Close must still report success")

	conn, err := net.Dial("unix", path)
	require.NoError(t, err, "the winner's socket must still be dialable after the loser's Close ran")
	conn.Close()
}
