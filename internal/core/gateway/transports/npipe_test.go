//go:build !windows

package transports_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
)

func TestNPipe_Listen_UnsupportedOffWindows(t *testing.T) {
	_, err := transports.NewNPipe(`\\.\pipe\quiver-test`).Listen()

	assert.ErrorIs(t, err, transports.ErrPipeUnsupported)
}

func TestDialPipe_UnsupportedOffWindows(t *testing.T) {
	_, err := transports.DialPipe(context.Background(), `\\.\pipe\quiver-test`)

	assert.ErrorIs(t, err, transports.ErrPipeUnsupported)
}
