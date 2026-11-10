package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type counter struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type increment struct {
	id string
	by int
}

func (c increment) AggregateID() string       { return c.id }
func (c increment) EventName() string         { return "counter.incremented." + c.id }
func (c increment) ShouldSnapshot() bool      { return true }
func (c increment) Validate(_ *counter) error { return nil }
func (c increment) EmitEvent(cur *counter) counter {
	next := counter{Name: c.id}
	if cur != nil {
		next = *cur
	}
	next.Count += c.by
	return next
}

func copyLegacyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"events.db", "snapshots.db"} {
		data, err := os.ReadFile(filepath.Join("testdata", "asynx-v0.8.1", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
	}
	return dir
}

func openLegacy(t *testing.T, dir string) asynx.Asynx[counter] {
	t.Helper()
	es, err := NewEventStore(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	ss, err := NewSnapshotStore(filepath.Join(dir, "snapshots.db"))
	require.NoError(t, err)
	ax, err := asynx.New[counter]().WithEventStore(es).WithSnapshotStore(ss).Build()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ax.Shutdown(context.Background())
		_ = es.Close()
		_ = ss.Close()
	})
	return ax
}

func TestEventStore_LegacyV08Database_ReadsExistingAggregates(t *testing.T) {
	ax := openLegacy(t, copyLegacyFixture(t))
	ctx := context.Background()

	got, err := ax.Get(ctx, "alpha")
	require.NoError(t, err)
	assert.Equal(t, counter{Name: "alpha", Count: 6}, got)

	got, err = ax.Get(ctx, "beta")
	require.NoError(t, err)
	assert.Equal(t, 6, got.Count)
}

func TestEventStore_LegacyV08Database_AcceptsNewCommandsAfterHistory(t *testing.T) {
	ax := openLegacy(t, copyLegacyFixture(t))
	ctx := context.Background()

	evt, err := ax.SendWait(ctx, increment{id: "alpha", by: 4})
	require.NoError(t, err)
	assert.Equal(t, int64(4), evt.Version)

	got, err := ax.Get(ctx, "alpha")
	require.NoError(t, err)
	assert.Equal(t, 10, got.Count)
}

func TestEventStore_LegacyV08Database_ForgetErasesLegacyAggregate(t *testing.T) {
	ax := openLegacy(t, copyLegacyFixture(t))
	ctx := context.Background()

	require.NoError(t, ax.Forget(ctx, "beta"))

	_, err := ax.Get(ctx, "beta")
	require.ErrorIs(t, err, asynxModels.ErrNotFound)

	got, err := ax.Get(ctx, "alpha")
	require.NoError(t, err)
	assert.Equal(t, 6, got.Count)
}

func TestEventStore_LegacyV08Database_SurvivesRestart(t *testing.T) {
	dir := copyLegacyFixture(t)
	ctx := context.Background()

	first := openLegacy(t, dir)
	_, err := first.SendWait(ctx, increment{id: "alpha", by: 1})
	require.NoError(t, err)
	require.NoError(t, first.Shutdown(ctx))

	second := openLegacy(t, dir)
	got, err := second.Get(ctx, "alpha")
	require.NoError(t, err)
	assert.Equal(t, 7, got.Count)
}
