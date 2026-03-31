package strategies

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

var _ Strategy = NewUPnP()

func TestUPnP_Name(
	t *testing.T,
) {
	s := NewUPnP()
	assert.Equal(t, "UPnP", s.Name())
}

func TestUPnP_Available(
	t *testing.T,
) {
	s := NewUPnP()
	assert.False(t, s.Available(context.Background()))
}

func TestUPnP_Forward(
	t *testing.T,
) {
	s := NewUPnP()
	err := s.Forward(context.Background(), 8080, ports.ProtocolTCP)
	assert.NoError(t, err)
}

func TestUPnP_Reverse(
	t *testing.T,
) {
	s := NewUPnP()
	err := s.Reverse(context.Background(), 8080, ports.ProtocolTCP)
	assert.NoError(t, err)
}
