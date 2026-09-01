package gateway_test

import (
	"net"
	"os"
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
