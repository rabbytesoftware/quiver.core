package testutil

import (
	"os"
	"runtime"
	"testing"
)

// RequireUnix skips a test that cannot hold on Windows.
//
// The CLI's daemon transport is a unix socket end to end — the default server
// is unix://~/.quiver/quiver.sock, the client dials AF_UNIX, and the daemon
// manager probes a socket file. Windows needs a named pipe instead, which is
// not implemented, so these tests are asserting behaviour the platform does
// not have rather than behaviour that is broken.
//
// Tests about $HOME land here too: os.UserHomeDir reads USERPROFILE on
// Windows, so clearing HOME does not produce the failure they exercise.
func RequireUnix(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the CLI is unix-only: its transport is a unix socket, not a named pipe")
	}
}

// RequireUnprivileged skips a test whose premise is an operation the OS
// refuses.
//
// Root bypasses the permission bits such a test sets up, so the operation
// succeeds and there is no error to assert. Windows does not model these
// permissions the same way at all. Neither is a bug in the code under test.
func RequireUnprivileged(t *testing.T) {
	t.Helper()

	RequireUnix(t)

	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits; nothing to assert")
	}
}
