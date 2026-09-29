package gateway_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
)

func TestNew_SocketScheme_AutoPath(t *testing.T) {
	f, err := os.CreateTemp("", "qv-*.sock")
	require.NoError(t, err)
	path := f.Name()
	f.Close()
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })

	ln, err := gateway.New(config.API{Host: "unix://" + path})
	require.NoError(t, err)
	defer ln.Close()

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	conn.Close()
}

func TestNew_TCPScheme(t *testing.T) {
	ln, err := gateway.New(config.API{Host: "tcp://127.0.0.1:0"})
	require.NoError(t, err)
	defer ln.Close()

	addr := ln.Addr().String()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	conn.Close()
}

func TestNew_UnknownScheme(t *testing.T) {
	_, err := gateway.New(config.API{Host: "grpc://localhost:1234"})
	assert.Error(t, err)
}

func TestNew_InvalidURI(t *testing.T) {
	_, err := gateway.New(config.API{Host: "://broken"})
	assert.Error(t, err)
}

func TestSocketPath_EmptyResolvesToHome(t *testing.T) {
	path := gateway.SocketPath("")
	assert.Contains(t, path, "quiver.sock")
}

func TestSocketPath_OverrideIsUsed(t *testing.T) {
	path := gateway.SocketPath("/custom/path.sock")
	assert.Equal(t, "/custom/path.sock", path)
}

func TestScheme_TCP(t *testing.T) {
	scheme, authority, err := gateway.Scheme("tcp://0.0.0.0:40257")
	require.NoError(t, err)
	assert.Equal(t, "tcp", scheme)
	assert.Equal(t, "0.0.0.0:40257", authority)
}

func TestScheme_Unix(t *testing.T) {
	scheme, authority, err := gateway.Scheme("unix:///custom/quiver.sock")
	require.NoError(t, err)
	assert.Equal(t, "unix", scheme)
	assert.Equal(t, "/custom/quiver.sock", authority)
}

func TestScheme_InvalidURI_ReturnsError(t *testing.T) {
	_, _, err := gateway.Scheme("not-a-uri")
	assert.Error(t, err)
}

func TestScheme_NPipe(t *testing.T) {
	scheme, authority, err := gateway.Scheme("npipe://quiver")
	require.NoError(t, err)
	assert.Equal(t, "npipe", scheme)
	assert.Equal(t, "quiver", authority)
}

func TestPipePath_EmptyResolvesToQuiver(t *testing.T) {
	assert.Equal(t, `\\.\pipe\quiver`, gateway.PipePath(""))
}

func TestPipePath_NameIsUsed(t *testing.T) {
	assert.Equal(t, `\\.\pipe\custom`, gateway.PipePath("custom"))
}

func TestLocalSocket_MatchesPlatform(t *testing.T) {
	got := gateway.LocalSocket("/home/u/.quiver")

	if runtime.GOOS == "windows" {
		assert.Equal(t, gateway.PipePath(""), got)
		return
	}
	assert.Equal(t, filepath.Join("/home/u/.quiver", "quiver.sock"), got)
}

func TestLocalURI_TableDriven(t *testing.T) {
	testCases := []struct {
		name   string
		socket string
		want   string
	}{
		{name: "pipe path", socket: `\\.\pipe\quiver`, want: "npipe://quiver"},
		{name: "unix path", socket: "/tmp/quiver.sock", want: "unix:///tmp/quiver.sock"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, gateway.LocalURI(tc.socket))
		})
	}
}

func TestDial_UnixSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	defer ln.Close()

	conn, err := gateway.Dial(context.Background(), path)
	require.NoError(t, err)
	conn.Close()
}

func TestDial_MissingSocketErrors(t *testing.T) {
	_, err := gateway.Dial(context.Background(), filepath.Join(t.TempDir(), "missing.sock"))
	assert.Error(t, err)
}

func TestNew_NPipeScheme_UnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are supported here")
	}

	_, err := gateway.New(config.API{Host: "npipe://quiver-test"})

	assert.Error(t, err)
}
