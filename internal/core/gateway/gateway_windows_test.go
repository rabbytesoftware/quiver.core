//go:build windows

package gateway_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
)

// dialAccepted dials pipe while a goroutine accepts the connection: go-winio
// only readies the next pipe instance inside Accept, so a dial with nobody
// accepting keeps retrying ERROR_PIPE_BUSY until its context expires.
func dialAccepted(
	t *testing.T,
	host string,
	pipe string,
) {
	t.Helper()

	ln, err := gateway.New(config.API{Host: host})
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		if conn, err := ln.Accept(); err == nil {
			conn.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := gateway.Dial(ctx, pipe)
	require.NoError(t, err)
	conn.Close()
}

func TestNew_NPipeScheme_NamedPipe(t *testing.T) {
	dialAccepted(t, "npipe://quiver-gateway-test", gateway.PipePath("quiver-gateway-test"))
}

func TestNew_UnixDefaultResolvesToPipeOnWindows(t *testing.T) {
	dialAccepted(t, "unix://", gateway.PipePath(""))
}
