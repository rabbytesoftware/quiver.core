package strategies

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

var _ Strategy = NewNATPMP()

func TestNATPMP_Name(
	t *testing.T,
) {
	s := NewNATPMP()
	assert.Equal(t, "NAT-PMP", s.Name())
}

func TestNATPMP_Available(
	t *testing.T,
) {
	s := NewNATPMP()
	assert.False(t, s.Available(context.Background()))
}

func TestNATPMP_Forward(
	t *testing.T,
) {
	s := NewNATPMP()
	err := s.Forward(context.Background(), 8080, ports.ProtocolUDP)
	assert.NoError(t, err)
}

func TestNATPMP_Reverse(
	t *testing.T,
) {
	s := NewNATPMP()
	err := s.Reverse(context.Background(), 8080, ports.ProtocolUDP)
	assert.NoError(t, err)
}
