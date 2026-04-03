package ports

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/stretchr/testify/assert"
)

func TestPortAllocation_ZeroValue(
	t *testing.T,
) {
	var alloc PortAllocation
	assert.Equal(t, 0, alloc.Port)
	assert.Equal(t, netbridge.Protocol(""), alloc.Protocol)
	assert.Equal(t, "", alloc.OwnerKey)
	assert.False(t, alloc.Forwarded)
}

func TestPortAllocation_FieldAssignment(
	t *testing.T,
) {
	alloc := PortAllocation{
		Port:      8080,
		Protocol:  netbridge.ProtocolTCP,
		OwnerKey:  "owner-1",
		Forwarded: true,
	}

	assert.Equal(t, 8080, alloc.Port)
	assert.Equal(t, netbridge.ProtocolTCP, alloc.Protocol)
	assert.Equal(t, "owner-1", alloc.OwnerKey)
	assert.True(t, alloc.Forwarded)
}
