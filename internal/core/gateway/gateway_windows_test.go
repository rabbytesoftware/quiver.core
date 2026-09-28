//go:build windows

package gateway_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
)

func TestNew_NPipeScheme_NamedPipe(t *testing.T) {
	ln, err := gateway.New(config.API{Host: "npipe://quiver-gateway-test"})
	require.NoError(t, err)
	defer ln.Close()

	conn, err := gateway.Dial(context.Background(), gateway.PipePath("quiver-gateway-test"))
	require.NoError(t, err)
	conn.Close()
}

func TestNew_UnixDefaultResolvesToPipeOnWindows(t *testing.T) {
	ln, err := gateway.New(config.API{Host: "unix://"})
	require.NoError(t, err)
	defer ln.Close()

	conn, err := gateway.Dial(context.Background(), gateway.PipePath(""))
	require.NoError(t, err)
	conn.Close()
}
