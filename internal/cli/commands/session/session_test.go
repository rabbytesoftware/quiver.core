package session_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/testutil"
)

func newCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func newFlags(t *testing.T, server string) *session.Flags {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: server}, true))
	return &session.Flags{Config: cfgPath}
}

func TestClient_TCPNoToken_BuildsClientWithoutBooting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":{"version":"1.0.0","build_id":"abc123","api":{"supported":["v0"],"latest":"v0"}}}`))
	}))
	defer srv.Close()

	booted := false
	sess := session.New(session.Deps{
		IsTTYFunc:    func() bool { return true },
		EnsureDaemon: func(context.Context) error { booted = true; return nil },
	}, newFlags(t, srv.URL))

	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	require.NoError(t, err)
	assert.False(t, booted, "tcp:// contexts must never trigger EnsureDaemon")
}

func TestClient_UnixScheme_BootsDaemon(t *testing.T) {
	booted := false
	sess := session.New(session.Deps{
		IsTTYFunc:    func() bool { return false },
		EnsureDaemon: func(context.Context) error { booted = true; return nil },
	}, newFlags(t, "unix:///tmp/does-not-matter.sock"))

	_, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	assert.True(t, booted)
}

func TestClient_TCPAutoPairOn401_PersistsToken(t *testing.T) {
	pairCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v0/auth/pairing":
			pairCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"code":"ABC","expires_at":"2026-09-16T15:00:00Z"}}`))
		case r.URL.Path == "/v0/auth/pairing/redeem":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"token":"fresh-token"}}`))
		case r.Header.Get("Authorization") != "Bearer fresh-token":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"missing or malformed bearer token"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"data":{"version":"1.0.0","build_id":"abc123","api":{"supported":["v0"],"latest":"v0"}}}`))
		}
	}))
	defer srv.Close()

	flags := newFlags(t, srv.URL)
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, flags)

	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, pairCalls)

	cfg, err := config.Load(flags.Config)
	require.NoError(t, err)
	ctx, err := cfg.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", ctx.Token)
}

func TestClient_ExplicitServerFlag_NeverPersistsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/auth/pairing":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"code":"ABC","expires_at":"2026-09-16T15:00:00Z"}}`))
		case "/v0/auth/pairing/redeem":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"token":"fresh-token"}}`))
		default:
			if r.Header.Get("Authorization") != "Bearer fresh-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"success":false,"error":"missing or malformed bearer token"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"data":{"version":"1.0.0","build_id":"abc123","api":{"supported":["v0"],"latest":"v0"}}}`))
		}
	}))
	defer srv.Close()

	flags := newFlags(t, "unix:///unused.sock")
	flags.Server = srv.URL
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, flags)

	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	require.NoError(t, err)

	cfg, err := config.Load(flags.Config)
	require.NoError(t, err)
	ctx, err := cfg.Get("test")
	require.NoError(t, err)
	assert.Empty(t, ctx.Token, "an ephemeral --server override has no context to persist into")
}

func TestSpinner_NonTTY_RunsFnDirectly(t *testing.T) {
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return false }}, &session.Flags{})
	ran := false
	require.NoError(t, sess.Spinner(newCmd(), "loading", func() error { ran = true; return nil }))
	assert.True(t, ran)
}

func TestIsTTY_DelegatesToDeps(t *testing.T) {
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, &session.Flags{})
	assert.True(t, sess.IsTTY())
}

// ─── coverage: remaining branches ────────────────────────────────────────────

func TestIsTTY_NilFunc_ReturnsFalse(t *testing.T) {
	sess := session.New(session.Deps{}, &session.Flags{})
	assert.False(t, sess.IsTTY())
}

func TestConfig_EmptyPath_UsesDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	sess := session.New(session.Deps{}, &session.Flags{})
	cfg, err := sess.Config()
	require.NoError(t, err)
	assert.Equal(t, config.LocalContextName, cfg.ActiveName())
}

func TestConfig_EmptyPath_DefaultPathErrors(t *testing.T) {
	testutil.RequireUnix(t)
	t.Setenv("HOME", "")

	sess := session.New(session.Deps{}, &session.Flags{})
	_, err := sess.Config()
	assert.Error(t, err)
}

func TestSpinner_TTY_RunsFnWithSpinner(t *testing.T) {
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, &session.Flags{})
	ran := false
	require.NoError(t, sess.Spinner(newCmd(), "loading", func() error { ran = true; return nil }))
	assert.True(t, ran)
}

func TestClient_ConfigLoadError_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not: [valid yaml"), 0o600))

	sess := session.New(session.Deps{IsTTYFunc: func() bool { return false }}, &session.Flags{Config: cfgPath})
	_, err := sess.Client(context.Background(), newCmd())
	assert.Error(t, err)
}

func TestClient_ResolveError_ReturnsError(t *testing.T) {
	flags := newFlags(t, "tcp://example.invalid:1")
	flags.Context = "does-not-exist"

	sess := session.New(session.Deps{IsTTYFunc: func() bool { return false }}, flags)
	_, err := sess.Client(context.Background(), newCmd())
	assert.Error(t, err)
}

func TestUnixClient_EnsureDaemonError_ReturnsError(t *testing.T) {
	sentinel := errors.New("boot failed")
	sess := session.New(session.Deps{
		IsTTYFunc:    func() bool { return false },
		EnsureDaemon: func(context.Context) error { return sentinel },
	}, newFlags(t, "unix:///tmp/does-not-matter.sock"))

	_, err := sess.Client(context.Background(), newCmd())
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestClient_TCPWithCachedToken_SkipsPairing(t *testing.T) {
	pairCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v0/auth/pairing":
			pairCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"code":"ABC","expires_at":"2026-09-16T15:00:00Z"}}`))
		case r.Header.Get("Authorization") != "Bearer cached-token":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"missing or malformed bearer token"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"success":true,"data":{"version":"1.0.0","build_id":"abc123","api":{"supported":["v0"],"latest":"v0"}}}`))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cli.yaml")
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.NoError(t, cfg.Add(config.Context{Name: "test", Server: srv.URL, Token: "cached-token"}, true))
	flags := &session.Flags{Config: cfgPath}

	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, flags)
	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, pairCalls, "a cached token must be used directly without re-pairing")
}

func TestClient_PairingGenerateFails_ReturnsOriginal401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/auth/pairing":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"missing or malformed bearer token"}`))
		}
	}))
	defer srv.Close()

	flags := newFlags(t, srv.URL)
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, flags)

	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	assert.Error(t, err)
}

func TestClient_PairingRedeemFails_ReturnsOriginal401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/auth/pairing":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"success":true,"data":{"code":"ABC","expires_at":"2026-09-16T15:00:00Z"}}`))
		case "/v0/auth/pairing/redeem":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"error":"missing or malformed bearer token"}`))
		}
	}))
	defer srv.Close()

	flags := newFlags(t, srv.URL)
	sess := session.New(session.Deps{IsTTYFunc: func() bool { return true }}, flags)

	cli, err := sess.Client(context.Background(), newCmd())
	require.NoError(t, err)
	_, err = cli.Versions(context.Background())
	assert.Error(t, err)
}
