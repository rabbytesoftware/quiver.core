package netbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

type alwaysAvailableStrategy struct {
	reverseCalledWith []int
}

func (s *alwaysAvailableStrategy) Name() string { return "mock" }

func (s *alwaysAvailableStrategy) Available(
	_ context.Context,
) bool {
	return true
}

func (s *alwaysAvailableStrategy) Forward(
	_ context.Context,
	_ int,
	_ ports.Protocol,
) error {
	return nil
}

func (s *alwaysAvailableStrategy) Reverse(
	_ context.Context,
	port int,
	_ ports.Protocol,
) error {
	s.reverseCalledWith = append(s.reverseCalledWith, port)
	return nil
}

type errFindByOwnerReadModel struct {
	store.PortStore
	err error
}

func (e *errFindByOwnerReadModel) FindByOwner(
	_ string,
) ([]ports.PortAllocation, error) {
	return nil, e.err
}

func buildNetbridgeWithStrategy(
	t *testing.T,
	strategy strategies.Strategy,
) *netbridgeImpl {
	t.Helper()

	nb, err := NewBuilder().Build(context.Background())
	require.NoError(t, err)

	impl := nb.(*netbridgeImpl)
	impl.strategies = []strategies.Strategy{strategy}
	return impl
}

func buildNetbridge(
	t *testing.T,
) *netbridgeImpl {
	t.Helper()

	nb, err := NewBuilder().Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl, ok := nb.(*netbridgeImpl)
	require.True(t, ok)
	return impl
}

func TestBuilder_BuildSucceeds(
	t *testing.T,
) {
	nb, err := NewBuilder().Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithStore(
	t *testing.T,
) {
	custom := store.NewPortMemory()
	nb, err := NewBuilder().WithStore(custom).Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl := nb.(*netbridgeImpl)
	assert.Equal(t, custom, impl.readModel)
}

func TestBuilder_WithDatabasePath(
	t *testing.T,
) {
	nb, err := NewBuilder().WithDatabasePath(":memory:").Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithDatabasePath_InvalidPath(
	t *testing.T,
) {
	_, err := NewBuilder().WithDatabasePath("/nonexistent/dir/ports.db").Build(context.Background())
	assert.Error(t, err)
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
	assert.GreaterOrEqual(t, port, ephemeralPortStart)
}

func TestAllocate_TCPUDPProtocol(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", ports.ProtocolTCPUDP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, ephemeralPortStart)
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
	strat := &alwaysAvailableStrategy{}
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
	strat := &alwaysAvailableStrategy{}
	nb := buildNetbridgeWithStrategy(t, strat)
	ctx := context.Background()

	const preferred = 54700
	port, err := nb.Allocate(ctx, "owner-rev", ports.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)

	nb.waitForProjection()

	err = nb.DeallocateByOwner(ctx, "owner-rev")
	require.NoError(t, err)

	assert.Equal(t, []int{preferred}, strat.reverseCalledWith)
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
	nb.readModel = &errFindByOwnerReadModel{
		PortStore: nb.readModel,
		err:       errors.New("read model failure"),
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
	assert.GreaterOrEqual(t, port2, ephemeralPortStart)
	assert.LessOrEqual(t, port2, ephemeralPortEnd)
}
