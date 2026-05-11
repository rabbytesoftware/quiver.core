package client_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"quiver-cli/client"
)

// Compile-time assertion: *FakeClient must satisfy QuiverClient.
var _ client.QuiverClient = (*client.FakeClient)(nil)

func TestFakeClient_UnsetFns_ReturnZeroValues(t *testing.T) {
	ctx := context.Background()
	f := &client.FakeClient{}

	items, err := f.ArrowList(ctx, true)
	assert.NoError(t, err)
	assert.Nil(t, items)

	detail, err := f.ArrowGet(ctx, "ns")
	assert.NoError(t, err)
	assert.Nil(t, detail)

	manifest, err := f.ArrowGetManifest(ctx, "ns")
	assert.NoError(t, err)
	assert.Nil(t, manifest)

	assert.NoError(t, f.ArrowAdd(ctx, "ns"))
	assert.NoError(t, f.ArrowUpdate(ctx, "ns"))
	assert.NoError(t, f.ArrowRemove(ctx, "ns"))
	assert.NoError(t, f.ArrowSeed(ctx, "ns", nil))

	vr, err := f.ArrowValidate(ctx, "ns", nil)
	assert.NoError(t, err)
	assert.Nil(t, vr)

	rt, err := f.RuntimeGet(ctx, "ns")
	assert.NoError(t, err)
	assert.Nil(t, rt)

	rts, err := f.RuntimeList(ctx)
	assert.NoError(t, err)
	assert.Nil(t, rts)

	cols, err := f.CollectionList(ctx)
	assert.NoError(t, err)
	assert.Nil(t, cols)

	col, err := f.CollectionGet(ctx, "ns")
	assert.NoError(t, err)
	assert.Nil(t, col)

	assert.NoError(t, f.CollectionAdd(ctx, "ns"))
	assert.NoError(t, f.CollectionUpdate(ctx, "ns"))
	assert.NoError(t, f.CollectionRemove(ctx, "ns"))

	health, err := f.Health(ctx)
	assert.NoError(t, err)
	assert.Nil(t, health)
}

func TestFakeClient_UnsetLifecycleFns_ReturnClosedEmptyChannel(t *testing.T) {
	ctx := context.Background()
	f := &client.FakeClient{}

	for _, ch := range []func() (<-chan client.ArrowRuntime, error){
		func() (<-chan client.ArrowRuntime, error) { return f.Install(ctx, "ns", nil) },
		func() (<-chan client.ArrowRuntime, error) { return f.Uninstall(ctx, "ns", nil) },
		func() (<-chan client.ArrowRuntime, error) { return f.Run(ctx, "ns", nil) },
		func() (<-chan client.ArrowRuntime, error) { return f.Stop(ctx, "ns") },
		func() (<-chan client.ArrowRuntime, error) { return f.Update(ctx, "ns") },
		func() (<-chan client.ArrowRuntime, error) { return f.RunMethod(ctx, "ns", "custom", nil) },
		func() (<-chan client.ArrowRuntime, error) { return f.WatchRuntime(ctx, "ns") },
	} {
		got, err := ch()
		require.NoError(t, err)
		require.NotNil(t, got)
		var snapshots []client.ArrowRuntime
		for s := range got {
			snapshots = append(snapshots, s)
		}
		assert.Empty(t, snapshots)
	}
}

func TestFakeClient_SetFn_IsCalled(t *testing.T) {
	ctx := context.Background()
	called := false

	f := &client.FakeClient{
		ArrowListFn: func(_ context.Context, userInstalled bool) ([]client.ArrowListItem, error) {
			called = true
			assert.True(t, userInstalled)
			return []client.ArrowListItem{{Namespace: "github.com/foo/bar"}}, nil
		},
	}

	items, err := f.ArrowList(ctx, true)
	require.NoError(t, err)
	assert.True(t, called)
	require.Len(t, items, 1)
	assert.Equal(t, "github.com/foo/bar", items[0].Namespace)
}

func TestFakeClient_InstallFn_StreamsSnapshots(t *testing.T) {
	ctx := context.Background()
	expected := []client.ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
		{Namespace: "github.com/foo/bar", State: "ready"},
	}

	f := &client.FakeClient{
		InstallFn: func(_ context.Context, ns string, _ map[string]string) (<-chan client.ArrowRuntime, error) {
			assert.Equal(t, "github.com/foo/bar", ns)
			return client.StreamOf(expected...), nil
		},
	}

	ch, err := f.Install(ctx, "github.com/foo/bar", nil)
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for s := range ch {
		got = append(got, s)
	}
	assert.Equal(t, expected, got)
}

func TestStreamOf_DeliversSnapshotsInOrder(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "a", State: "installing"},
		{Namespace: "a", State: "ready"},
	}

	ch := client.StreamOf(snapshots...)

	var got []client.ArrowRuntime
	for s := range ch {
		got = append(got, s)
	}
	assert.Equal(t, snapshots, got)
}

func TestStreamOf_Empty_ReturnsClosedChannel(t *testing.T) {
	ch := client.StreamOf()
	var got []client.ArrowRuntime
	for s := range ch {
		got = append(got, s)
	}
	assert.Empty(t, got)
}
