package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/auth"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func newSession(t *testing.T, server string) session.Session {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))
	return session.New(session.Deps{IsTTYFunc: func() bool { return false }}, &session.Flags{Config: cfgPath})
}

// assertMutation checks that out is a JSON output.Mutation payload
// reporting action against subject, with an RFC3339 timestamp — the same
// contract every other mutation command in the CLI carries under -o json.
func assertMutation(t *testing.T, out, action, subject string) {
	t.Helper()

	var m output.Mutation
	require.NoError(t, json.Unmarshal([]byte(out), &m), "mutation payload: %s", out)

	assert.Equal(t, output.Action(action), m.Action)
	assert.Equal(t, subject, m.Subject)

	_, err := time.Parse(time.RFC3339, m.At)
	assert.NoError(t, err, "at must be RFC3339, got %q", m.At)
}

func TestDevicesList_Success_PrintsDevices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"d1","label":"cli@host","state":"active","paired_at":"2026-09-16T15:00:00Z","last_seen_at":"2026-09-16T15:05:00Z"}]}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"devices", "list"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "d1")
}

func TestDevicesList_Empty_PrintsEmptyDoc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"devices", "list"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "[]")
}

func TestDevicesList_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"devices", "list"})
	require.Error(t, root.Execute())
}

func TestDevicesList_TableOutput_RendersColumns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"d1","label":"cli@host","state":"active","paired_at":"2026-09-16T15:00:00Z","last_seen_at":"2026-09-16T15:05:00Z"}]}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "table"}))
	root := cmds.Cmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"devices", "list"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "LABEL")
	assert.Contains(t, out.String(), "cli@host")
}

func TestDevicesRevoke_Success_PrintsMutation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/devices/d1", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"devices", "revoke", "d1"})
	require.NoError(t, root.Execute())
	assertMutation(t, out.String(), "revoke", "d1")
}

func TestDevicesRevoke_Success_TableOutputRendersLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/devices/d1", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "table"}))
	root := cmds.Cmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"devices", "revoke", "d1"})
	require.NoError(t, root.Execute())
	assert.Contains(t, out.String(), "revoked d1")
}

func TestDevicesRevoke_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"not found"}`))
	}))
	defer srv.Close()

	cmds := auth.New(newSession(t, srv.URL), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"devices", "revoke", "d1"})
	require.Error(t, root.Execute())
}

func TestDevicesRevoke_ClientResolveError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir + "/cli.yaml")
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: "unix:///nonexistent"}, true))
	sess := session.New(
		session.Deps{IsTTYFunc: func() bool { return false }},
		&session.Flags{Config: dir + "/cli.yaml", Context: "missing"},
	)

	cmds := auth.New(sess, runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"devices", "revoke", "d1"})
	require.Error(t, root.Execute())
}

func TestDevicesRevoke_NoArgs_ReturnsUsageError(t *testing.T) {
	cmds := auth.New(newSession(t, "unix:///nonexistent"), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"devices", "revoke"})
	require.Error(t, root.Execute())
}

func TestCmd_HasAuthAndDevicesSubcommands(t *testing.T) {
	cmds := auth.New(newSession(t, "unix:///nonexistent"), runner.New(&runner.Flags{Output: "json"}))
	root := cmds.Cmd()
	assert.Equal(t, "auth", root.Use)

	devices, _, err := root.Find([]string{"devices"})
	require.NoError(t, err)
	assert.Equal(t, "devices", devices.Use)
}
