package recommendationinternal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	recommendationinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store"
)

var fixedNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type refreshFixture struct {
	browser   *mocks.Browser
	store     store.Store
	db        *gorm.DB
	hooks     *recommendationinternal.Hooks
	refreshed int
}

func newFixture(t *testing.T, answers ...mocks.BrowseAnswer) *refreshFixture {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := store.New(db)
	require.NoError(t, err)

	f := &refreshFixture{browser: &mocks.Browser{Answers: answers}, store: st, db: db, hooks: &recommendationinternal.Hooks{}}
	require.NoError(t, f.hooks.OnRefreshed(func(context.Context) { f.refreshed++ }))
	return f
}

func (f *refreshFixture) refresher(shelves []recommendationinternal.Shelf, minEntries int) recommendationinternal.Refresher {
	return recommendationinternal.NewRefresher(recommendationinternal.RefresherConfig{
		Browser:    f.browser,
		Store:      f.store,
		Shelves:    shelves,
		Budget:     30,
		MinEntries: minEntries,
		Now:        func() time.Time { return fixedNow },
		Hooks:      f.hooks,
	})
}

func popular(limit int) []recommendationinternal.Shelf {
	return []recommendationinternal.Shelf{{
		ID:    "popular",
		Title: "Popular",
		Limit: limit,
		Sources: []recommendationinternal.Source{
			{Host: "github", Sort: "stars", MinStars: 500, MaxStars: 20000, PushedWithin: 90 * 24 * time.Hour},
		},
	}}
}

func answer(order []string, emit ...discovery.Result) mocks.BrowseAnswer {
	return mocks.BrowseAnswer{Emit: emit, Outcome: mocks.Outcome(order...)}
}

func snapshotOf(t *testing.T, f *refreshFixture, id string) store.Snapshot {
	t.Helper()
	snap, err := f.store.Snapshot(context.Background(), id)
	require.NoError(t, err)
	return snap
}

func namespaces(entries []store.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Namespace)
	}
	return out
}

func TestRefresher_Refresh_BuildsTheShelfInSearchOrder(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/one", "github.com/a/two", "github.com/a/three"},
		mocks.Result("github.com/a/three", 3, "github.com"),
		mocks.Result("github.com/a/one", 1, "github.com"),
		mocks.Result("github.com/a/two", 2, "github.com"),
	))

	require.NoError(t, f.refresher(popular(24), 1).Refresh(context.Background()))

	snap := snapshotOf(t, f, "popular")
	assert.Equal(t, []store.Entry{
		{Namespace: "github.com/a/one", Stars: 1, Source: "github.com"},
		{Namespace: "github.com/a/two", Stars: 2, Source: "github.com"},
		{Namespace: "github.com/a/three", Stars: 3, Source: "github.com"},
	}, snap.Entries)
	assert.True(t, fixedNow.Equal(snap.RefreshedAt))
	assert.Equal(t, 1, f.refreshed)
}

func TestRefresher_Refresh_SendsSourcesBudgetAndWantWithAFullPage(t *testing.T) {
	f := newFixture(t, answer([]string{}))

	require.NoError(t, f.refresher(popular(24), 0).Refresh(context.Background()))

	require.Len(t, f.browser.Requests, 1)
	req := f.browser.Requests[0]
	assert.Equal(t, 30, req.Budget)
	assert.Equal(t, 24, req.Want)
	assert.Equal(t, []discovery.BrowseSource{
		{Host: "github", Sort: "stars", MinStars: 500, MaxStars: 20000, PushedWithin: 90 * 24 * time.Hour, Limit: 100},
	}, req.Sources)
}

func TestRefresher_Refresh_AsksForAFullPageWhateverTheShelfLimit(t *testing.T) {
	for _, limit := range []int{3, 12, 60} {
		f := newFixture(t, answer([]string{}))

		require.NoError(t, f.refresher(popular(limit), 0).Refresh(context.Background()))

		assert.Equal(t, 100, f.browser.Requests[0].Sources[0].Limit)
	}
}

func TestRefresher_Refresh_TruncatesToTheShelfLimit(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/one", "github.com/a/two", "github.com/a/three"},
		mocks.Result("github.com/a/one", 1, "github.com"),
		mocks.Result("github.com/a/two", 2, "github.com"),
		mocks.Result("github.com/a/three", 3, "github.com"),
	))

	require.NoError(t, f.refresher(popular(2), 1).Refresh(context.Background()))

	assert.Equal(t, []string{"github.com/a/one", "github.com/a/two"}, namespaces(snapshotOf(t, f, "popular").Entries))
}

func TestRefresher_Refresh_SkipsCandidatesThatNeverVerified(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/one", "github.com/a/two"},
		mocks.Result("github.com/a/two", 2, "github.com"),
	))

	require.NoError(t, f.refresher(popular(24), 1).Refresh(context.Background()))

	assert.Equal(t, []string{"github.com/a/two"}, namespaces(snapshotOf(t, f, "popular").Entries))
}

func TestRefresher_Refresh_ShortListIsPaddedFromThePreviousSnapshot(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/new", "github.com/a/old1"},
		mocks.Result("github.com/a/new", 9, "github.com"),
		mocks.Result("github.com/a/old1", 8, "github.com"),
	))
	require.NoError(t, f.store.Replace(context.Background(), "popular", fixedNow.Add(-time.Hour), []store.Entry{
		{Namespace: "github.com/a/old1"},
		{Namespace: "github.com/a/old2"},
		{Namespace: "github.com/a/old3"},
		{Namespace: "github.com/a/old4"},
	}))

	require.NoError(t, f.refresher(popular(24), 4).Refresh(context.Background()))

	assert.Equal(t,
		[]string{"github.com/a/new", "github.com/a/old1", "github.com/a/old2", "github.com/a/old3"},
		namespaces(snapshotOf(t, f, "popular").Entries),
		"padding stops at the minimum and never repeats an arrow already present",
	)
}

func TestRefresher_Refresh_LongEnoughListIsNotPadded(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/new"},
		mocks.Result("github.com/a/new", 9, "github.com"),
	))
	require.NoError(t, f.store.Replace(context.Background(), "popular", fixedNow, []store.Entry{{Namespace: "github.com/a/old"}}))

	require.NoError(t, f.refresher(popular(24), 1).Refresh(context.Background()))

	assert.Equal(t, []string{"github.com/a/new"}, namespaces(snapshotOf(t, f, "popular").Entries))
}

func TestRefresher_Refresh_NothingFoundKeepsThePreviousSnapshotUntouched(t *testing.T) {
	f := newFixture(t, answer([]string{}))
	old := fixedNow.Add(-48 * time.Hour)
	require.NoError(t, f.store.Replace(context.Background(), "popular", old, []store.Entry{{Namespace: "github.com/a/old"}}))

	require.NoError(t, f.refresher(popular(24), 0).Refresh(context.Background()))

	snap := snapshotOf(t, f, "popular")
	assert.Equal(t, []string{"github.com/a/old"}, namespaces(snap.Entries))
	assert.True(t, old.Equal(snap.RefreshedAt))
	assert.Zero(t, f.refreshed, "nothing changed, so nothing is announced")
}

func TestRefresher_Refresh_NothingFoundAndNoSnapshot_RecordsAnEmptyShelf(t *testing.T) {
	f := newFixture(t, answer([]string{}))

	require.NoError(t, f.refresher(popular(24), 6).Refresh(context.Background()))

	snap := snapshotOf(t, f, "popular")
	assert.Empty(t, snap.Entries)
	assert.True(t, fixedNow.Equal(snap.RefreshedAt))
}

func TestRefresher_Refresh_BrowseError_KeepsThePreviousSnapshot(t *testing.T) {
	f := newFixture(t, mocks.BrowseAnswer{Err: errors.New("boom")})
	require.NoError(t, f.store.Replace(context.Background(), "popular", fixedNow.Add(-time.Hour), []store.Entry{{Namespace: "github.com/a/old"}}))

	err := f.refresher(popular(24), 0).Refresh(context.Background())

	require.Error(t, err)
	assert.Equal(t, []string{"github.com/a/old"}, namespaces(snapshotOf(t, f, "popular").Entries))
	assert.Zero(t, f.refreshed)
}

func TestRefresher_Refresh_EveryProviderFailing_KeepsThePreviousSnapshot(t *testing.T) {
	failed := mocks.BrowseAnswer{Outcome: discovery.Outcome{
		Providers: []discovery.ProviderOutcome{{Host: "github.com", Reason: discovery.ReasonRateLimited}},
	}}
	f := newFixture(t, failed)
	require.NoError(t, f.store.Replace(context.Background(), "popular", fixedNow.Add(-time.Hour), []store.Entry{{Namespace: "github.com/a/old"}}))

	err := f.refresher(popular(24), 0).Refresh(context.Background())

	require.Error(t, err)
	assert.Equal(t, []string{"github.com/a/old"}, namespaces(snapshotOf(t, f, "popular").Entries))
}

func TestRefresher_Refresh_OneProviderFailingAnotherAnswering_StillSwaps(t *testing.T) {
	partial := answer([]string{"github.com/a/one"}, mocks.Result("github.com/a/one", 1, "github.com"))
	partial.Outcome.Providers = append(partial.Outcome.Providers, discovery.ProviderOutcome{Host: "gitlab.com", Reason: discovery.ReasonError})
	f := newFixture(t, partial)

	require.NoError(t, f.refresher(popular(24), 0).Refresh(context.Background()))

	assert.Equal(t, []string{"github.com/a/one"}, namespaces(snapshotOf(t, f, "popular").Entries))
}

func TestRefresher_Refresh_CancelledMidPass_KeepsThePreviousSnapshot(t *testing.T) {
	f := newFixture(t, answer(
		[]string{"github.com/a/new"},
		mocks.Result("github.com/a/new", 1, "github.com"),
	))
	ctx, cancel := context.WithCancel(context.Background())
	f.browser.OnBrowse = func(context.Context) { cancel() }
	require.NoError(t, f.store.Replace(context.Background(), "popular", fixedNow.Add(-time.Hour), []store.Entry{{Namespace: "github.com/a/old"}}))

	err := f.refresher(popular(24), 0).Refresh(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"github.com/a/old"}, namespaces(snapshotOf(t, f, "popular").Entries))
	assert.Zero(t, f.refreshed)
}

func TestRefresher_Refresh_CancelledBeforeAShelf_StopsWithoutBrowsing(t *testing.T) {
	f := newFixture(t, answer([]string{}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := f.refresher(popular(24), 0).Refresh(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, f.browser.Calls)
}

func TestRefresher_Refresh_OneShelfFailingDoesNotStopTheNext(t *testing.T) {
	shelves := append(popular(24), recommendationinternalShelf("fresh"))
	f := newFixture(t,
		mocks.BrowseAnswer{Err: errors.New("boom")},
		answer([]string{"github.com/a/one"}, mocks.Result("github.com/a/one", 1, "github.com")),
	)

	err := f.refresher(shelves, 0).Refresh(context.Background())

	require.Error(t, err)
	assert.Empty(t, snapshotOf(t, f, "popular").Entries)
	assert.Equal(t, []string{"github.com/a/one"}, namespaces(snapshotOf(t, f, "fresh").Entries))
	assert.Equal(t, 1, f.refreshed)
}

func recommendationinternalShelf(id string) recommendationinternal.Shelf {
	return recommendationinternal.Shelf{
		ID:      id,
		Title:   id,
		Limit:   24,
		Sources: []recommendationinternal.Source{{Host: "github", Sort: "updated"}},
	}
}

func TestRefresher_Refresh_SnapshotReadFailure_IsReported(t *testing.T) {
	f := newFixture(t, answer([]string{}))
	require.NoError(t, f.db.Migrator().DropTable("recommendation_shelves"))

	err := f.refresher(popular(24), 0).Refresh(context.Background())

	require.Error(t, err)
}

func TestRefresher_Refresh_ReplaceFailure_IsReported(t *testing.T) {
	f := newFixture(t, answer([]string{"github.com/a/one"}, mocks.Result("github.com/a/one", 1, "github.com")))
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("fail_entries", func(tx *gorm.DB) {
		_ = tx.AddError(errors.New("disk full"))
	}))

	err := f.refresher(popular(24), 0).Refresh(context.Background())

	require.Error(t, err)
	assert.Zero(t, f.refreshed)
}

func TestBrowser_RepeatsTheLastAnswerPastTheEnd(t *testing.T) {
	b := &mocks.Browser{Answers: []mocks.BrowseAnswer{answer([]string{"github.com/a/one"})}}

	for range 2 {
		outcome, err := b.Browse(context.Background(), discovery.BrowseRequest{}, func(discovery.Result) {})
		require.NoError(t, err)
		assert.Len(t, outcome.Order, 1)
	}
	assert.Equal(t, 2, b.Calls)
}

func TestRefresher_Refresh_OutcomeWithNoProviders_IsNotTreatedAsAnOutage(t *testing.T) {
	f := newFixture(t, mocks.BrowseAnswer{})

	require.NoError(t, f.refresher(popular(24), 0).Refresh(context.Background()))

	assert.True(t, fixedNow.Equal(snapshotOf(t, f, "popular").RefreshedAt))
}
