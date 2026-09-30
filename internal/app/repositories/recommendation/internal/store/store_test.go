package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	return db
}

func newStore(t *testing.T) (store.Store, *gorm.DB) {
	t.Helper()
	db := openDB(t)
	s, err := store.New(db)
	require.NoError(t, err)
	return s, db
}

func TestNew_NilDB_ReturnsError(t *testing.T) {
	_, err := store.New(nil)

	require.Error(t, err)
}

func TestNew_ClosedDB_MigrateFails(t *testing.T) {
	db := openDB(t)
	require.NoError(t, adapterSQLite.CloseDB(db))

	_, err := store.New(db)

	require.Error(t, err)
}

func TestSnapshot_UnknownShelf_IsEmptyNotAnError(t *testing.T) {
	s, _ := newStore(t)

	got, err := s.Snapshot(context.Background(), "nope")

	require.NoError(t, err)
	assert.True(t, got.RefreshedAt.IsZero())
	assert.Empty(t, got.Entries)
}

func TestReplace_ThenSnapshot_RoundTripsInRankOrder(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	entries := []store.Entry{
		{Namespace: "github.com/acme/c", Stars: 3, Source: "github.com"},
		{Namespace: "github.com/acme/a", Stars: 9, Source: "github.com"},
		{Namespace: "github.com/acme/b", Stars: 1, Source: "gitlab.com"},
	}

	require.NoError(t, s.Replace(ctx, "popular", at, entries))
	got, err := s.Snapshot(ctx, "popular")

	require.NoError(t, err)
	assert.Equal(t, entries, got.Entries)
	assert.True(t, at.Equal(got.RefreshedAt))
}

func TestReplace_SecondCall_ReplacesTheWholeShelf(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	first := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)
	require.NoError(t, s.Replace(ctx, "popular", first, []store.Entry{{Namespace: "github.com/acme/a"}, {Namespace: "github.com/acme/b"}}))

	require.NoError(t, s.Replace(ctx, "popular", second, []store.Entry{{Namespace: "github.com/acme/z"}}))
	got, err := s.Snapshot(ctx, "popular")

	require.NoError(t, err)
	assert.Equal(t, []store.Entry{{Namespace: "github.com/acme/z"}}, got.Entries)
	assert.True(t, second.Equal(got.RefreshedAt))
}

func TestReplace_EmptyEntries_RecordsAnEmptyShelf(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.Replace(ctx, "popular", at, nil))
	got, err := s.Snapshot(ctx, "popular")

	require.NoError(t, err)
	assert.Empty(t, got.Entries)
	assert.True(t, at.Equal(got.RefreshedAt))
}

func TestReplace_OtherShelvesAreUntouched(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.Replace(ctx, "popular", at, []store.Entry{{Namespace: "github.com/acme/a"}}))

	require.NoError(t, s.Replace(ctx, "fresh", at, []store.Entry{{Namespace: "github.com/acme/b"}}))
	got, err := s.Snapshot(ctx, "popular")

	require.NoError(t, err)
	assert.Equal(t, []store.Entry{{Namespace: "github.com/acme/a"}}, got.Entries)
}

func TestReplace_FailureMidTransaction_KeepsThePreviousSnapshot(t *testing.T) {
	s, db := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	old := []store.Entry{{Namespace: "github.com/acme/old"}}
	require.NoError(t, s.Replace(ctx, "popular", at, old))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_entries", func(tx *gorm.DB) {
		if tx.Statement.Table == "recommendation_entries" {
			_ = tx.AddError(errors.New("disk full"))
		}
	}))

	err := s.Replace(ctx, "popular", at.Add(time.Hour), []store.Entry{{Namespace: "github.com/acme/new"}})
	require.Error(t, err)
	require.NoError(t, db.Callback().Create().Remove("fail_entries"))
	got, readErr := s.Snapshot(ctx, "popular")

	require.NoError(t, readErr)
	assert.Equal(t, old, got.Entries)
	assert.True(t, at.Equal(got.RefreshedAt))
}

func TestStore_ClosedDB_EveryOperationFails(t *testing.T) {
	s, db := newStore(t)
	require.NoError(t, adapterSQLite.CloseDB(db))
	ctx := context.Background()

	_, snapErr := s.Snapshot(ctx, "popular")
	replaceErr := s.Replace(ctx, "popular", time.Now(), nil)
	retainErr := s.Retain(ctx, []string{"popular"})

	assert.Error(t, snapErr)
	assert.Error(t, replaceErr)
	assert.Error(t, retainErr)
}

func TestSnapshot_EntriesQueryFails_ReturnsError(t *testing.T) {
	s, db := newStore(t)
	require.NoError(t, db.Migrator().DropTable("recommendation_entries"))

	_, err := s.Snapshot(context.Background(), "popular")

	require.Error(t, err)
}

func TestRetain_DropsShelvesTheConfigurationNoLongerHas(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.Replace(ctx, "popular", at, []store.Entry{{Namespace: "github.com/acme/a"}}))
	require.NoError(t, s.Replace(ctx, "gone", at, []store.Entry{{Namespace: "github.com/acme/b"}}))

	require.NoError(t, s.Retain(ctx, []string{"popular"}))
	kept, keptErr := s.Snapshot(ctx, "popular")
	dropped, droppedErr := s.Snapshot(ctx, "gone")

	require.NoError(t, keptErr)
	require.NoError(t, droppedErr)
	assert.Len(t, kept.Entries, 1)
	assert.Empty(t, dropped.Entries)
	assert.True(t, dropped.RefreshedAt.IsZero())
}

func TestRetain_NoShelves_DropsEverything(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.Replace(ctx, "popular", time.Now(), []store.Entry{{Namespace: "github.com/acme/a"}}))

	require.NoError(t, s.Retain(ctx, nil))
	got, err := s.Snapshot(ctx, "popular")

	require.NoError(t, err)
	assert.Empty(t, got.Entries)
}

func TestRetain_EntryDeleteFails_ReturnsError(t *testing.T) {
	s, db := newStore(t)
	require.NoError(t, db.Migrator().DropTable("recommendation_entries"))

	require.Error(t, s.Retain(context.Background(), []string{"popular"}))
	require.Error(t, s.Retain(context.Background(), nil))
}

func TestRetain_ShelfDeleteFails_ReturnsError(t *testing.T) {
	s, db := newStore(t)
	require.NoError(t, db.Migrator().DropTable("recommendation_shelves"))

	require.Error(t, s.Retain(context.Background(), []string{"popular"}))
	require.Error(t, s.Retain(context.Background(), nil))
}

func TestSnapshot_ShelfQueryFails_ReturnsError(t *testing.T) {
	s, db := newStore(t)
	require.NoError(t, db.Migrator().DropTable("recommendation_shelves"))

	_, err := s.Snapshot(context.Background(), "popular")

	require.Error(t, err)
}
