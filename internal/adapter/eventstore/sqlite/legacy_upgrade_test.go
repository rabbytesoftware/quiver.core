package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	glebarez "github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

const legacyEventsDDL = "CREATE TABLE `events` (`aggregate_id` text,`version` integer,`data` blob NOT NULL,PRIMARY KEY (`aggregate_id`,`version`))"

const legacySnapshotsDDL = "CREATE TABLE `snapshots` (`aggregate_id` text,`version` integer NOT NULL,`data` blob NOT NULL,PRIMARY KEY (`aggregate_id`))"

func legacyEvent(
	id string,
	version int,
	patches string,
) string {
	return fmt.Sprintf(
		`{"id":"%s-%d","event_name":"counter.incremented.%s","version":%d,"schema_version":1,`+
			`"occurred_at":"2026-11-08T22:56:45Z","patches":[%s]}`,
		id, version, id, version, patches,
	)
}

func legacySnapshot(
	id string,
) string {
	return fmt.Sprintf(`{"version":3,"schema_version":1,"state":{"name":"%s","count":6}}`, id)
}

func execAll(
	t *testing.T,
	path string,
	statements ...[]any,
) {
	t.Helper()
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range statements {
		require.NoError(t, db.Exec(stmt[0].(string), stmt[1:]...).Error)
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// copyLegacyFixture writes the databases an asynx v0.8.1 daemon leaves behind
// (no receipts or deliveries tables): aggregates alpha and beta, each
// incremented by 1, 2 and 3.
func copyLegacyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	events := [][]any{{legacyEventsDDL}}
	snapshots := [][]any{{legacySnapshotsDDL}}
	for _, id := range []string{"alpha", "beta"} {
		first := `{"value":1,"op":"replace","path":"/count"},{"value":"` + id + `","op":"replace","path":"/name"}`
		for version, patches := range []string{
			first,
			`{"value":3,"op":"replace","path":"/count"}`,
			`{"value":6,"op":"replace","path":"/count"}`,
		} {
			events = append(events, []any{
				"INSERT INTO events (aggregate_id, version, data) VALUES (?, ?, ?)",
				"events:" + id, version + 1, []byte(legacyEvent(id, version+1, patches)),
			})
		}
		snapshots = append(snapshots, []any{
			"INSERT INTO snapshots (aggregate_id, version, data) VALUES (?, 3, ?)",
			id, []byte(legacySnapshot(id)),
		})
	}
	execAll(t, filepath.Join(dir, "events.db"), events...)
	execAll(t, filepath.Join(dir, "snapshots.db"), snapshots...)
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
