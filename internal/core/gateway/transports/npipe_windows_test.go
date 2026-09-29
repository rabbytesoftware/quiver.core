//go:build windows

package transports_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
)

func pipePath(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`\\.\pipe\quiver-test-%d`, testPipeSeq.Add(1))
}

func TestNPipe_ListenAndDial_RoundTrip(t *testing.T) {
	path := pipePath(t)
	ln, err := transports.NewNPipe(path).Listen()
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4)
		_, _ = conn.Read(buf)
		accepted <- buf
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := transports.DialPipe(ctx, path)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	assert.Equal(t, []byte("ping"), <-accepted)
}

func TestNPipe_Listen_SecondDaemonGetsErrDaemonRunning(t *testing.T) {
	path := pipePath(t)
	ln, err := transports.NewNPipe(path).Listen()
	require.NoError(t, err)
	defer ln.Close()

	_, err = transports.NewNPipe(path).Listen()

	assert.ErrorIs(t, err, transports.ErrDaemonRunning)
}

func TestNPipe_Listen_AfterCloseCanRebind(t *testing.T) {
	path := pipePath(t)
	ln, err := transports.NewNPipe(path).Listen()
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	again, err := transports.NewNPipe(path).Listen()
	require.NoError(t, err)
	_ = again.Close()
}

func TestNPipe_Listen_InvalidPathErrors(t *testing.T) {
	_, err := transports.NewNPipe(`\\.\pipe\`).Listen()

	assert.Error(t, err)
}

func TestDialPipe_MissingPipeErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := transports.DialPipe(ctx, pipePath(t))

	assert.Error(t, err)
}
