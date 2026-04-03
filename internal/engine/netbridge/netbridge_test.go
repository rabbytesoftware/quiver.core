package netbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

func buildNetbridgeWithStrategy(
	t *testing.T,
	strategy strategies.Strategy,
) *netbridgeService {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	nb, err := New().WithEventStore(es).Build(context.Background())
	require.NoError(t, err)

	impl := nb.(*netbridgeService)
	impl.strategies = []strategies.Strategy{strategy}
	return impl
}

func buildNetbridge(
	t *testing.T,
) *netbridgeService {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	nb, err := New().WithEventStore(es).Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl, ok := nb.(*netbridgeService)
	require.True(t, ok)
	return impl
}

func TestBuilder_BuildSucceeds(
	t *testing.T,
) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	nb, err := New().WithEventStore(es).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithStore(
	t *testing.T,
) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	custom := store.NewPortMemory()
	nb, err := New().WithStore(custom).WithEventStore(es).Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl := nb.(*netbridgeService)
	assert.Equal(t, custom, impl.readModel)
}

func TestBuilder_WithDatabasePath(
	t *testing.T,
) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	nb, err := New().WithEventStore(es).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithDatabasePath_InvalidPath(
	t *testing.T,
) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	nb, err := New().WithEventStore(es).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestAllocate_ReturnsValidPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", ports.ProtocolTCP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, 1)
	assert.LessOrEqual(t, port, 65535)
}

func TestAllocate_HonorsPreferredPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	const preferred = 54321
	port, err := nb.Allocate(context.Background(), "owner1", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)
}

func TestAllocate_UDPProtocol(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", ports.ProtocolUDP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, testEphemeralPortStart)
}

func TestAllocate_TCPUDPProtocol(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", ports.ProtocolTCPUDP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, testEphemeralPortStart)
}

func TestAllocate_ReturnsErrPortOutOfRange(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	ctx := context.Background()

	cases := []int{-1, 99999}
	for _, preferred := range cases {
		_, err := nb.Allocate(ctx, "owner1", ports.ProtocolTCP, preferred)
		assert.ErrorIs(t, err, ErrPortOutOfRange, "expected ErrPortOutOfRange for preferred=%d", preferred)
	}
}

func TestAllocate_WithActiveStrategy_SetsForwarded(
	t *testing.T,
) {
	strat := &mocks.AlwaysAvailableStrategy{}
	nb := buildNetbridgeWithStrategy(t, strat)
	ctx := context.Background()

	const preferred = 54600
	port, err := nb.Allocate(ctx, "owner-fwd", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)

	nb.waitForProjection()

	alloc, err := nb.readModel.FindByPort(port)
	require.NoError(t, err)
	require.NotNil(t, alloc)
	assert.True(t, alloc.Forwarded)
}

func TestAllocate_SendErrorOnCancelledContext(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := nb.Allocate(ctx, "owner-cancel", ports.ProtocolTCP, 0)
	assert.Error(t, err)
}

func TestDeallocateByOwner_ReleasesAllPorts(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	ctx := context.Background()

	const (
		preferred1 = 54400
		preferred2 = 54401
		ownerKey   = "owner-dealloc"
	)

	port1, err := nb.Allocate(ctx, ownerKey, ports.ProtocolTCP, preferred1)
	require.NoError(t, err)
	assert.Equal(t, preferred1, port1)

	port2, err := nb.Allocate(ctx, ownerKey, ports.ProtocolTCP, preferred2)
	require.NoError(t, err)
	assert.Equal(t, preferred2, port2)

	nb.waitForProjection()

	err = nb.DeallocateByOwner(ctx, ownerKey)
	require.NoError(t, err)
	nb.waitForProjection()

	realloc1, err := nb.Allocate(ctx, ownerKey, ports.ProtocolTCP, preferred1)
	require.NoError(t, err)
	assert.Equal(t, preferred1, realloc1)

	realloc2, err := nb.Allocate(ctx, ownerKey, ports.ProtocolTCP, preferred2)
	require.NoError(t, err)
	assert.Equal(t, preferred2, realloc2)
}

func TestDeallocateByOwner_NoOpForUnknownOwner(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	err := nb.DeallocateByOwner(context.Background(), "unknown-owner")
	assert.NoError(t, err)
}

func TestDeallocateByOwner_WithActiveStrategy_CallsReverse(
	t *testing.T,
) {
	strat := &mocks.AlwaysAvailableStrategy{}
	nb := buildNetbridgeWithStrategy(t, strat)
	ctx := context.Background()

	const preferred = 54700
	port, err := nb.Allocate(ctx, "owner-rev", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)

	nb.waitForProjection()

	err = nb.DeallocateByOwner(ctx, "owner-rev")
	require.NoError(t, err)

	assert.Equal(t, []int{preferred}, strat.ReverseCalledWith)
}

func TestDeallocateByOwner_SendErrorOnCancelledContext(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	const preferred = 54800
	port, allocErr := nb.Allocate(context.Background(), "owner-cancel", ports.ProtocolTCP, preferred)
	require.NoError(t, allocErr)
	assert.Equal(t, preferred, port)
	nb.waitForProjection()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := nb.DeallocateByOwner(ctx, "owner-cancel")
	assert.Error(t, err)
}

func TestDeallocateByOwner_FindByOwnerError(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	nb.readModel = &mocks.ErrFindByOwnerReadModel{
		PortStore: nb.readModel,
		Err:       errors.New("read model failure"),
	}

	err := nb.DeallocateByOwner(context.Background(), "some-owner")
	assert.Error(t, err)
}

func TestAllocate_SamePreferredPortTwiceGetsDifferentPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	ctx := context.Background()

	const preferred = 54500

	port1, err := nb.Allocate(ctx, "owner-a", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port1)

	nb.waitForProjection()

	port2, err := nb.Allocate(ctx, "owner-b", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.NotEqual(t, preferred, port2)
	assert.GreaterOrEqual(t, port2, testEphemeralPortStart)
	assert.LessOrEqual(t, port2, testEphemeralPortEnd)
}
