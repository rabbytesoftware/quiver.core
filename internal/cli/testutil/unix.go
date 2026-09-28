package testutil

import (
	"os"
	"runtime"
	"testing"
)

// RequireUnix skips a test that cannot hold on Windows.
//
// These tests build their own unix socket fixtures, bind sockets at paths they
// choose, or rely on POSIX behaviour, none of which Windows has. The
// production Windows transport is a named pipe, covered by its own tests.
//
// Tests about $HOME land here too: os.UserHomeDir reads USERPROFILE on
// Windows, so clearing HOME does not produce the failure they exercise.
func RequireUnix(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("test relies on unix sockets or POSIX behaviour")
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
