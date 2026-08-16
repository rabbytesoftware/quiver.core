package netbridge

import (
	"context"
	"errors"
	"strconv"
	"testing"

	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/strategies"
)

func buildNetbridgeWithStrategy(
	t *testing.T,
	strategy strategies.Strategy,
) *netbridgeService {
	t.Helper()

	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().WithEventStore(es).WithSnapshotStore(ss).WithStrategies([]strategies.Strategy{strategy}).Build(context.Background())
	require.NoError(t, err)

	impl, ok := nb.(*netbridgeService)
	require.True(t, ok)
	return impl
}

func buildNetbridge(
	t *testing.T,
) *netbridgeService {
	t.Helper()

	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().WithEventStore(es).WithSnapshotStore(ss).WithStrategies([]strategies.Strategy{}).Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl, ok := nb.(*netbridgeService)
	require.True(t, ok)
	return impl
}

func TestBuilder_BuildSucceeds(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().WithEventStore(es).WithSnapshotStore(ss).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithStore(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	custom := store.NewPortMemory()
	nb, err := New().WithStore(custom).WithEventStore(es).WithSnapshotStore(ss).Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl := nb.(*netbridgeService)
	assert.Equal(t, custom, impl.readModel)
}

func TestBuilder_WithDatabasePath(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().WithEventStore(es).WithSnapshotStore(ss).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestBuilder_WithDatabasePath_InvalidPath(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().WithEventStore(es).WithSnapshotStore(ss).Build(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, nb)
}

func TestAllocate_ReturnsValidPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", netbridge.ProtocolTCP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, 1)
	assert.LessOrEqual(t, port, 65535)
}

func TestAllocate_HonorsPreferredPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	const preferred = 54321
	port, err := nb.Allocate(context.Background(), "owner1", netbridge.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)
}

func TestAllocate_UDPProtocol(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", netbridge.ProtocolUDP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, testEphemeralPortStart)
}

func TestAllocate_TCPUDPProtocol(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	port, err := nb.Allocate(context.Background(), "owner1", netbridge.ProtocolTCPUDP, 0)
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
		_, err := nb.Allocate(ctx, "owner1", netbridge.ProtocolTCP, preferred)
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
	port, err := nb.Allocate(ctx, "owner-fwd", netbridge.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port)

	nb.waitForProjection()

	alloc, err := nb.readModel.FindByPort(ctx, port)
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

	_, err := nb.Allocate(ctx, "owner-cancel", netbridge.ProtocolTCP, 0)
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

	port1, err := nb.Allocate(ctx, ownerKey, netbridge.ProtocolTCP, preferred1)
	require.NoError(t, err)
	assert.Equal(t, preferred1, port1)

	port2, err := nb.Allocate(ctx, ownerKey, netbridge.ProtocolTCP, preferred2)
	require.NoError(t, err)
	assert.Equal(t, preferred2, port2)

	nb.waitForProjection()

	err = nb.DeallocateByOwner(ctx, ownerKey)
	require.NoError(t, err)
	nb.waitForProjection()

	realloc1, err := nb.Allocate(ctx, ownerKey, netbridge.ProtocolTCP, preferred1)
	require.NoError(t, err)
	assert.Equal(t, preferred1, realloc1)

	realloc2, err := nb.Allocate(ctx, ownerKey, netbridge.ProtocolTCP, preferred2)
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
	port, err := nb.Allocate(ctx, "owner-rev", netbridge.ProtocolTCP, preferred)
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
	port, allocErr := nb.Allocate(context.Background(), "owner-cancel", netbridge.ProtocolTCP, preferred)
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

func TestBuilder_WithEphemeralPortRange_AppliedToBuild(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithStrategies([]strategies.Strategy{}).
		WithEphemeralPortRange(50000, 50100).
		Build(context.Background())
	require.NoError(t, err)
	require.NotNil(t, nb)

	impl := nb.(*netbridgeService)
	assert.Equal(t, 50000, impl.portStart)
	assert.Equal(t, 50100, impl.portEnd)
}

func TestBuilder_Build_MissingEventStore_ReturnsError(
	t *testing.T,
) {
	_, err := New().Build(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBuildFailed)
}

func TestBuilder_Build_MissingSnapshotStoreReturnsError(
	t *testing.T,
) {
	_, err := New().WithEventStore(asynxstore.New()).Build(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBuildFailed)
	assert.Contains(t, err.Error(), "missing SnapshotStore")
}

func TestAllocate_WithEphemeralPortRange_UsesCustomRange(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithStrategies([]strategies.Strategy{}).
		WithEphemeralPortRange(50200, 50300).
		Build(context.Background())
	require.NoError(t, err)

	// preferred=0 forces ephemeral allocation from the custom range.
	port, allocErr := nb.Allocate(context.Background(), "owner-custom", netbridge.ProtocolTCP, 0)
	require.NoError(t, allocErr)
	assert.GreaterOrEqual(t, port, 50200)
	assert.LessOrEqual(t, port, 50300)
}

func TestNetbridge_Allocate_WritesSnapshot(
	t *testing.T,
) {
	ss := asynxstore.NewSnapshots()
	nb, err := New().
		WithEventStore(asynxstore.New()).
		WithSnapshotStore(ss).
		WithStrategies([]strategies.Strategy{}).
		Build(context.Background())
	require.NoError(t, err)

	port, err := nb.Allocate(context.Background(), "owner-1", netbridge.ProtocolTCP, 50000)
	require.NoError(t, err)

	_, found, err := ss.Get(context.Background(), strconv.Itoa(port))
	require.NoError(t, err)
	assert.True(t, found, "allocate must persist a snapshot; without one every read cold-replays the port's full history forever")
}

func TestAllocate_SamePreferredPortTwiceGetsDifferentPort(
	t *testing.T,
) {
	nb := buildNetbridge(t)
	ctx := context.Background()

	const preferred = 54500

	port1, err := nb.Allocate(ctx, "owner-a", netbridge.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.Equal(t, preferred, port1)

	nb.waitForProjection()

	port2, err := nb.Allocate(ctx, "owner-b", netbridge.ProtocolTCP, preferred)
	require.NoError(t, err)
	assert.NotEqual(t, preferred, port2)
	assert.GreaterOrEqual(t, port2, testEphemeralPortStart)
	assert.LessOrEqual(t, port2, testEphemeralPortEnd)
}

func TestShutdown_DrainsAsynx(
	t *testing.T,
) {
	nb := buildNetbridge(t)

	require.NoError(t, nb.Shutdown(context.Background()))

	_, err := nb.Allocate(context.Background(), "owner-a", netbridge.ProtocolTCP, 0)
	assert.Error(t, err, "a drained aggregate must reject new allocations")
}

func buildNetbridgeNoForwarding(
	t *testing.T,
	strategy strategies.Strategy,
) *netbridgeService {
	t.Helper()

	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithStrategies([]strategies.Strategy{strategy}).
		WithForwarding(false).
		Build(context.Background())
	require.NoError(t, err)

	impl, ok := nb.(*netbridgeService)
	require.True(t, ok)
	return impl
}

func TestBuilder_ResolveForwarding_DefaultsToConfig(
	t *testing.T,
) {
	assert.Equal(t, config.GetNetbridge().Enabled, New().resolveForwarding())
}

func TestBuilder_ResolveForwarding_OptionOverridesConfig(
	t *testing.T,
) {
	testCases := []struct {
		name    string
		enabled bool
	}{
		{name: "explicitly enabled", enabled: true},
		{name: "explicitly disabled", enabled: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.enabled, New().WithForwarding(tc.enabled).resolveForwarding())
		})
	}
}

func TestBuilder_WithForwarding_DisabledDropsInjectedStrategies(
	t *testing.T,
) {
	nb := buildNetbridgeNoForwarding(t, &mocks.AlwaysAvailableStrategy{})

	assert.Empty(t, nb.resolveStrategies())
}

func TestBuilder_WithForwarding_EnabledKeepsInjectedStrategies(
	t *testing.T,
) {
	es := asynxstore.New()
	ss := asynxstore.NewSnapshots()

	nb, err := New().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithStrategies([]strategies.Strategy{&mocks.AlwaysAvailableStrategy{}}).
		WithForwarding(true).
		Build(context.Background())
	require.NoError(t, err)

	impl, ok := nb.(*netbridgeService)
	require.True(t, ok)
	assert.Len(t, impl.resolveStrategies(), 1)
}

// No preferred port is asked for: which port comes back is the allocator's
// business and depends on what the machine already has bound, while what this
// test is about is that nothing was forwarded.
func TestAllocate_ForwardingDisabled_AllocatesWithoutForwarding(
	t *testing.T,
) {
	nb := buildNetbridgeNoForwarding(t, &mocks.AlwaysAvailableStrategy{})
	ctx := context.Background()

	port, err := nb.Allocate(ctx, "owner-nofwd", netbridge.ProtocolTCP, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, nb.portStart)
	assert.LessOrEqual(t, port, nb.portEnd)

	nb.waitForProjection()

	alloc, err := nb.readModel.FindByPort(ctx, port)
	require.NoError(t, err)
	require.NotNil(t, alloc)
	assert.False(t, alloc.Forwarded)
}

func TestDeallocateByOwner_ForwardingDisabled_SkipsReverse(
	t *testing.T,
) {
	strat := &mocks.AlwaysAvailableStrategy{}
	nb := buildNetbridgeNoForwarding(t, strat)
	ctx := context.Background()

	port, err := nb.Allocate(ctx, "owner-nofwd-rev", netbridge.ProtocolTCP, 0)
	require.NoError(t, err)

	nb.waitForProjection()

	err = nb.DeallocateByOwner(ctx, "owner-nofwd-rev")
	require.NoError(t, err)
	nb.waitForProjection()

	assert.Empty(t, strat.ReverseCalledWith)

	// The port is free again, so asking for it by name gets it back.
	realloc, err := nb.Allocate(ctx, "owner-nofwd-rev", netbridge.ProtocolTCP, port)
	require.NoError(t, err)
	assert.Equal(t, port, realloc)
}
