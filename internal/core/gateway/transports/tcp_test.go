package transports_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
)

func TestTCPTransport_Listen(t *testing.T) {
	t.Run("binds to address and accepts connections", func(t *testing.T) {
		transport := transports.NewTCP("127.0.0.1:0")

		ln, err := transport.Listen()
		require.NoError(t, err)
		defer ln.Close()

		addr := ln.Addr().String()
		conn, err := net.Dial("tcp", addr)
		require.NoError(t, err)
		conn.Close()
	})

	t.Run("returns error on invalid address", func(t *testing.T) {
		transport := transports.NewTCP("invalid-address")

		_, err := transport.Listen()
		assert.Error(t, err)
	})
}
