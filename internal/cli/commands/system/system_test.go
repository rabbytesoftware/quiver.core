package system_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/system"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

// ─── fixtures ────────────────────────────────────────────────────────────────

// newSession builds a session.Session with an active context pointing at
// server, for a non-TTY invocation. Mirrors the pattern in
// commands/collection/collection_test.go.
func newSession(t *testing.T, server string) session.Session {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))

	return session.New(
		session.Deps{IsTTYFunc: func() bool { return false }},
		&session.Flags{Config: cfgPath},
	)
}

// fakeSystemDaemon serves the health/versions endpoints the system package's
// commands call.
type fakeSystemDaemon struct {
	// fail makes every request return 500, for daemon-error-propagation and
	// daemon-unreachable tests.
	fail bool
}

func (f *fakeSystemDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}

		switch r.URL.Path {
		case "/v0/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/versions":
			_, _ = w.Write([]byte(`{"success":true,"data":{"version":"26.5","build_id":"83",` +
				`"api":{"supported":["v0"],"latest":"v0"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"not found"}`))
		}
	})
}

// runSystem runs the system commands (health/version, both root-level)
// against sess.
func runSystem(t *testing.T, sess session.Session, version string, args ...string) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(system.New(sess, version).Cmd()...)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

// ─── Cmd ─────────────────────────────────────────────────────────────────────

func TestCmd_ReturnsHealthAndVersion(t *testing.T) {
	got := system.New(newSession(t, "unix:///unused.sock"), "test").Cmd()

	require.Len(t, got, 2)
	assert.Equal(t, "health", got[0].Use)
	assert.Equal(t, "version", got[1].Use)
}

// ─── health ──────────────────────────────────────────────────────────────────

func TestHealth_OK(t *testing.T) {
	srv := httptest.NewServer((&fakeSystemDaemon{}).handler())
	defer srv.Close()

	out, err := runSystem(t, newSession(t, srv.URL), "test", "health")
	require.NoError(t, err)
	assert.Contains(t, out, "daemon: ok")
}

func TestHealth_DaemonError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer((&fakeSystemDaemon{fail: true}).handler())
	defer srv.Close()

	_, err := runSystem(t, newSession(t, srv.URL), "test", "health")
	assert.Error(t, err)
}

func TestHealth_SessionError_ReturnsError(t *testing.T) {
	_, err := runSystem(t, newSession(t, "ftp://nope"), "test", "health")
	assert.Error(t, err)
}

// ─── version ─────────────────────────────────────────────────────────────────

func TestVersion_ShowsClientAndDaemon(t *testing.T) {
	srv := httptest.NewServer((&fakeSystemDaemon{}).handler())
	defer srv.Close()

	out, err := runSystem(t, newSession(t, srv.URL), "test", "version")
	require.NoError(t, err)
	assert.Contains(t, out, "client test")
	assert.Contains(t, out, "daemon 26.5")
}

func TestVersion_ClientOnlySkipsDaemon(t *testing.T) {
	srv := httptest.NewServer((&fakeSystemDaemon{}).handler())
	defer srv.Close()

	out, err := runSystem(t, newSession(t, srv.URL), "test", "version", "--client-only")
	require.NoError(t, err)
	assert.Contains(t, out, "client test")
	assert.NotContains(t, out, "26.5")
}

func TestVersion_DaemonUnreachableStillSucceeds(t *testing.T) {
	srv := httptest.NewServer((&fakeSystemDaemon{fail: true}).handler())
	defer srv.Close()

	out, err := runSystem(t, newSession(t, srv.URL), "test", "version")
	require.NoError(t, err)
	assert.Contains(t, out, "client test")
	assert.Contains(t, out, "daemon unreachable")
}

func TestVersion_SessionError_StillPrintsClientVersion(t *testing.T) {
	out, err := runSystem(t, newSession(t, "ftp://nope"), "test", "version")
	assert.Error(t, err)
	assert.Contains(t, out, "client test")
}
