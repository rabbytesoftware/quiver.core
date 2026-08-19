// Package testutil holds helpers shared by the CLI's tests.
package testutil

import (
	"os"
	"testing"
)

// SocketDir returns a directory short enough to hold a unix socket path.
//
// A unix socket address is capped by the size of sun_path — 104 bytes on
// Darwin, 108 on Linux — and the whole path counts, not just the filename.
// t.TempDir() on macOS lands under
// /var/folders/<hash>/T/<TestName><random>/001/, which on its own can exceed
// the cap; binding then fails with "bind: invalid argument" rather than
// anything that names the real problem.
//
// This is the same limit that breaks the real daemon when $HOME is long, so a
// test hitting it is reproducing a genuine constraint rather than an artefact
// of the harness.
func SocketDir(t *testing.T) string {
	t.Helper()
	RequireUnix(t)

	// /tmp is short and present on every unix. Falling back to the default
	// keeps this compiling and running where it is not.
	base := "/tmp"
	if _, err := os.Stat(base); err != nil {
		base = ""
	}

	dir, err := os.MkdirTemp(base, "q")
	if err != nil {
		t.Fatalf("testutil: socket dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}
