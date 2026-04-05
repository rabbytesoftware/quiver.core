package store

import (
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func TestNewPortMemory_ReturnsPortStore(
	t *testing.T,
) {
	rm := NewPortMemory()
	assert.NotNil(t, rm)
}

func TestPortStore_Save_FindByKey(
	t *testing.T,
) {
	rm := NewPortMemory()
	alloc := ports.PortAllocation{
		Port:      8080,
		Protocol:  netbridge.ProtocolTCP,
		OwnerKey:  "owner-1",
		Forwarded: false,
	}

	err := rm.Save(alloc)
	require.NoError(t, err)

	found, err := rm.FindByKey(8080)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, alloc, *found)
}

func TestPortStore_FindByPort(
	t *testing.T,
) {
	rm := NewPortMemory()
	alloc := ports.PortAllocation{
		Port:      8080,
		Protocol:  netbridge.ProtocolUDP,
		OwnerKey:  "owner-2",
		Forwarded: true,
	}

	err := rm.Save(alloc)
	require.NoError(t, err)

	found, err := rm.FindByPort(8080)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, alloc, *found)
}

func TestPortStore_FindByOwner(
	t *testing.T,
) {
	rm := NewPortMemory()
	alloc1 := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}
	alloc2 := ports.PortAllocation{Port: 8081, OwnerKey: "owner-1"}
	alloc3 := ports.PortAllocation{Port: 8082, OwnerKey: "owner-2"}

	require.NoError(t, rm.Save(alloc1))
	require.NoError(t, rm.Save(alloc2))
	require.NoError(t, rm.Save(alloc3))

	results, err := rm.FindByOwner("owner-1")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestPortStore_Delete(
	t *testing.T,
) {
	rm := NewPortMemory()
	alloc := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}

	require.NoError(t, rm.Save(alloc))
	require.NoError(t, rm.Delete(8080))

	found, err := rm.FindByKey(8080)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestPortStore_FindAll(
	t *testing.T,
) {
	rm := NewPortMemory()
	alloc1 := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}
	alloc2 := ports.PortAllocation{Port: 8081, OwnerKey: "owner-2"}

	require.NoError(t, rm.Save(alloc1))
	require.NoError(t, rm.Save(alloc2))

	results, err := rm.FindAll()
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestNewPortSQLite(
	t *testing.T,
) {
	rm, err := NewPortSQLite(":memory:")
	require.NoError(t, err)
	assert.NotNil(t, rm)
}

func TestPortStore_SQLite_Save_FindByKey(
	t *testing.T,
) {
	rm, err := NewPortSQLite(":memory:")
	require.NoError(t, err)

	alloc := ports.PortAllocation{
		Port:      9999,
		Protocol:  netbridge.ProtocolTCP,
		OwnerKey:  "owner-sqlite",
		Forwarded: true,
	}

	err = rm.Save(alloc)
	require.NoError(t, err)

	found, err := rm.FindByKey(9999)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, alloc, *found)
}

func TestPortStore_SQLite_FindByOwner(
	t *testing.T,
) {
	rm, err := NewPortSQLite(":memory:")
	require.NoError(t, err)

	alloc1 := ports.PortAllocation{Port: 9000, OwnerKey: "sqlite-owner"}
	alloc2 := ports.PortAllocation{Port: 9001, OwnerKey: "sqlite-owner"}
	alloc3 := ports.PortAllocation{Port: 9002, OwnerKey: "other-owner"}

	require.NoError(t, rm.Save(alloc1))
	require.NoError(t, rm.Save(alloc2))
	require.NoError(t, rm.Save(alloc3))

	results, err := rm.FindByOwner("sqlite-owner")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestNewPortSQLite_InvalidPath(
	t *testing.T,
) {
	_, err := NewPortSQLite("/invalid/path/that/does/not/exist/db.db")
	assert.Error(t, err)
}
