package ws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMessage(t *testing.T) {
	msg := NewMessage(MessageStatus, "ok")

	assert.Equal(t, MessageStatus, msg.Type)
	assert.Equal(t, "ok", msg.Payload)
	assert.False(t, msg.TimeStamp.IsZero())
}
