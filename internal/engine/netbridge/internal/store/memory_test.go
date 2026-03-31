package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func TestMemory_Save_And_FindByPort(
	t *testing.T,
) {
	rm := NewMemory()
	alloc := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1", Protocol: ports.ProtocolTCP}

	require.NoError(t, rm.Save(alloc))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, alloc, *result)
}

func TestMemory_FindByPort_NotFound(
	t *testing.T,
) {
	rm := NewMemory()

	result, err := rm.FindByPort(9999)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMemory_Save_Overwrite(
	t *testing.T,
) {
	rm := NewMemory()

	first := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}
	second := ports.PortAllocation{Port: 8080, OwnerKey: "owner-2"}

	require.NoError(t, rm.Save(first))
	require.NoError(t, rm.Save(second))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "owner-2", result.OwnerKey)
}

func TestMemory_Delete_Existing(
	t *testing.T,
) {
	rm := NewMemory()
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}))

	require.NoError(t, rm.Delete(8080))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestMemory_Delete_NonExisting(
	t *testing.T,
) {
	rm := NewMemory()
	assert.NoError(t, rm.Delete(9999))
}

func TestMemory_FindByOwner_MultipleResults(
	t *testing.T,
) {
	rm := NewMemory()
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}))
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8081, OwnerKey: "owner-1"}))
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8082, OwnerKey: "owner-2"}))

	results, err := rm.FindByOwner("owner-1")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestMemory_FindByOwner_NoMatch(
	t *testing.T,
) {
	rm := NewMemory()

	results, err := rm.FindByOwner("nobody")
	require.NoError(t, err)
	assert.Empty(t, results)
}
