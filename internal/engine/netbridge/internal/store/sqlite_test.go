package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func newTestSQLite(
	t *testing.T,
) Store {
	t.Helper()
	store, err := NewSQLite(":memory:")
	require.NoError(t, err)
	return store
}

func TestSQLite_Save_And_FindByPort(
	t *testing.T,
) {
	rm := newTestSQLite(t)
	alloc := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1", Protocol: ports.ProtocolTCP, Forwarded: true}

	require.NoError(t, rm.Save(alloc))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, alloc, *result)
}

func TestSQLite_FindByPort_NotFound(
	t *testing.T,
) {
	rm := newTestSQLite(t)

	result, err := rm.FindByPort(9999)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestSQLite_Save_Overwrite(
	t *testing.T,
) {
	rm := newTestSQLite(t)

	first := ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}
	second := ports.PortAllocation{Port: 8080, OwnerKey: "owner-2"}

	require.NoError(t, rm.Save(first))
	require.NoError(t, rm.Save(second))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "owner-2", result.OwnerKey)
}

func TestSQLite_Delete_Existing(
	t *testing.T,
) {
	rm := newTestSQLite(t)
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}))

	require.NoError(t, rm.Delete(8080))

	result, err := rm.FindByPort(8080)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestSQLite_Delete_NonExisting(
	t *testing.T,
) {
	rm := newTestSQLite(t)
	assert.NoError(t, rm.Delete(9999))
}

func TestSQLite_FindByOwner_MultipleResults(
	t *testing.T,
) {
	rm := newTestSQLite(t)
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8080, OwnerKey: "owner-1"}))
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8081, OwnerKey: "owner-1"}))
	require.NoError(t, rm.Save(ports.PortAllocation{Port: 8082, OwnerKey: "owner-2"}))

	results, err := rm.FindByOwner("owner-1")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestSQLite_FindByOwner_NoMatch(
	t *testing.T,
) {
	rm := newTestSQLite(t)

	results, err := rm.FindByOwner("nobody")
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSQLite_NewSQLite_InvalidPath(
	t *testing.T,
) {
	_, err := NewSQLite("/nonexistent/dir/ports.db")
	assert.Error(t, err)
}
