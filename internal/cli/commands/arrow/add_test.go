package arrow_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
)

type confirmDaemon struct {
	failStatus int

	mu    sync.Mutex
	calls []string
}

func (d *confirmDaemon) record(r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
}

func (d *confirmDaemon) recorded() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]string(nil), d.calls...)
}

func (d *confirmDaemon) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.record(r)
		w.Header().Set("Content-Type", "application/json")

		if d.failStatus != 0 {
			w.WriteHeader(d.failStatus)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}

		if r.URL.Query().Get("confirm") == "true" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}

		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"success":false,"error":"confirmation required"}`))
	})
}

func newTTYSession(t *testing.T, server string) session.Session {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))

	return session.New(
		session.Deps{IsTTYFunc: func() bool { return true }},
		&session.Flags{Config: cfgPath},
	)
}

func runArrowSession(
	t *testing.T,
	sess session.Session,
	stdin string,
	args ...string,
) (string, error) {
	t.Helper()

	cmds := arrow.New(sess, runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), err
}

func TestArrowAdd_ConfirmationRequired_YesFlagRetries(t *testing.T) {
	f := &confirmDaemon{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runArrowSession(t, newSession(t, srv.URL), "", "add", testNS, "--yes")

	require.NoError(t, err)
	calls := f.recorded()
	require.Len(t, calls, 2)
	assert.NotContains(t, calls[0], "confirm=true")
	assert.Contains(t, calls[1], "confirm=true")
}

func TestArrowAdd_ConfirmationRequired_TTYYesRetries(t *testing.T) {
	f := &confirmDaemon{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	out, err := runArrowSession(t, newTTYSession(t, srv.URL), "y\n", "add", testNS)

	require.NoError(t, err)
	assert.Contains(t, out, "Install anyway")
	require.Len(t, f.recorded(), 2)
	assert.Contains(t, f.recorded()[1], "confirm=true")
}

func TestArrowAdd_ConfirmationRequired_TTYNoAborts(t *testing.T) {
	f := &confirmDaemon{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runArrowSession(t, newTTYSession(t, srv.URL), "n\n", "add", testNS)

	require.Error(t, err)
	require.Len(t, f.recorded(), 1, "a declined confirmation must not retry")
}

func TestArrowAdd_ConfirmationRequired_NonTTYWithoutYesErrors(t *testing.T) {
	f := &confirmDaemon{}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runArrowSession(t, newSession(t, srv.URL), "", "add", testNS)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--yes")
	require.Len(t, f.recorded(), 1, "an unconfirmed non-TTY call must not retry")
}

func TestArrowAdd_OtherError_DoesNotRetry(t *testing.T) {
	f := &confirmDaemon{failStatus: http.StatusInternalServerError}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)

	_, err := runArrowSession(t, newSession(t, srv.URL), "", "add", testNS, "--yes")

	require.Error(t, err)
	require.Len(t, f.recorded(), 1, "a non-409 error must not be retried")
}
