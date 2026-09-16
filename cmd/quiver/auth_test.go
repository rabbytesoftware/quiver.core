package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runAuthCmd(t *testing.T, srvURL string, args ...string) (string, error) {
	t.Helper()

	root := newAuthCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs(append(args, "--host", "tcp://"+srvURL))

	err := root.Execute()
	return out.String(), err
}

func TestAuthGenerateCmd_InvalidHost_ReturnsError(t *testing.T) {
	cmd := newAuthGenerateCmd(strPtr("not-a-uri"))
	cmd.SetOut(&bytes.Buffer{})
	assert.Error(t, cmd.RunE(cmd, nil))
}

func TestAuthDevicesListCmd_InvalidHost_ReturnsError(t *testing.T) {
	cmd := newAuthDevicesListCmd(strPtr("not-a-uri"))
	cmd.SetOut(&bytes.Buffer{})
	assert.Error(t, cmd.RunE(cmd, nil))
}

func TestAuthDevicesRevokeCmd_InvalidHost_ReturnsError(t *testing.T) {
	cmd := newAuthDevicesRevokeCmd(strPtr("not-a-uri"))
	cmd.SetOut(&bytes.Buffer{})
	assert.Error(t, cmd.RunE(cmd, []string{"dev-1"}))
}

func strPtr(s string) *string { return &s }

func TestAuthGenerateCmd_UnreachableDaemon_ReturnsError(t *testing.T) {
	_, err := runAuthCmd(t, "127.0.0.1:1", "generate")
	assert.Error(t, err)
}

func TestAuthDevicesListCmd_UnreachableDaemon_ReturnsError(t *testing.T) {
	_, err := runAuthCmd(t, "127.0.0.1:1", "devices", "list")
	assert.Error(t, err)
}

func TestAuthDevicesRevokeCmd_UnreachableDaemon_ReturnsError(t *testing.T) {
	_, err := runAuthCmd(t, "127.0.0.1:1", "devices", "revoke", "dev-1")
	assert.Error(t, err)
}

func TestAuthGenerateCmd_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/pairing", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]string{"code": "482913", "expires_at": "2026-09-01T12:05:00Z"},
		})
	}))
	defer srv.Close()

	out, err := runAuthCmd(t, srv.Listener.Addr().String(), "generate")
	require.NoError(t, err)
	assert.Contains(t, out, "482913")
}

func TestAuthGenerateCmd_ServiceError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "reachable only from the daemon's own host"})
	}))
	defer srv.Close()

	_, err := runAuthCmd(t, srv.Listener.Addr().String(), "generate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon's own host")
}

func TestAuthDevicesListCmd_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/devices", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": []map[string]string{
				{"id": "dev-1", "label": "laptop", "state": "active", "last_seen_at": "2026-09-01T12:00:00Z"},
			},
		})
	}))
	defer srv.Close()

	out, err := runAuthCmd(t, srv.Listener.Addr().String(), "devices", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "dev-1")
	assert.Contains(t, out, "laptop")
}

func TestAuthDevicesListCmd_Empty_PrintsMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]string{}})
	}))
	defer srv.Close()

	out, err := runAuthCmd(t, srv.Listener.Addr().String(), "devices", "list")
	require.NoError(t, err)
	assert.Contains(t, out, "No paired devices")
}

func TestAuthDevicesListCmd_ServiceError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "boom"})
	}))
	defer srv.Close()

	_, err := runAuthCmd(t, srv.Listener.Addr().String(), "devices", "list")
	assert.Error(t, err)
}

func TestAuthDevicesRevokeCmd_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/devices/dev-1", r.URL.Path)
		assert.Equal(t, http.MethodDelete, r.Method)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer srv.Close()

	out, err := runAuthCmd(t, srv.Listener.Addr().String(), "devices", "revoke", "dev-1")
	require.NoError(t, err)
	assert.Contains(t, out, "Revoked device dev-1")
}

func TestAuthDevicesRevokeCmd_ServiceError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "not found"})
	}))
	defer srv.Close()

	_, err := runAuthCmd(t, srv.Listener.Addr().String(), "devices", "revoke", "missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestAuthDevicesRevokeCmd_RequiresExactlyOneArg(t *testing.T) {
	cmd := newAuthDevicesRevokeCmd(new(string))
	assert.Error(t, cmd.Args(cmd, []string{}))
	assert.Error(t, cmd.Args(cmd, []string{"a", "b"}))
	assert.NoError(t, cmd.Args(cmd, []string{"dev-1"}))
}

func TestAuthCmd_HasHostFlagAndSubcommands(t *testing.T) {
	cmd := newAuthCmd()
	assert.NotNil(t, cmd.PersistentFlags().Lookup("host"))

	generate, _, err := cmd.Find([]string{"generate"})
	require.NoError(t, err)
	assert.Equal(t, "generate", generate.Name())

	devices, _, err := cmd.Find([]string{"devices", "list"})
	require.NoError(t, err)
	assert.Equal(t, "list", devices.Name())
}

func TestRootCommand_HasAuthSubcommand(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"auth"})
	require.NoError(t, err)
	assert.Equal(t, "auth", cmd.Name())
}
