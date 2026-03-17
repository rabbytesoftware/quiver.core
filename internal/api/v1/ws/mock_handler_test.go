package ws_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api/v1/ws"
	"github.com/stretchr/testify/assert"
)

func TestNewMessage(t *testing.T) {
	msg := ws.NewMessage(ws.MessageStatus, "ok")

	assert.Equal(t, ws.MessageStatus, msg.Type)
	assert.Equal(t, "ok", msg.Payload)
	assert.False(t, msg.TimeStamp.IsZero())
}
