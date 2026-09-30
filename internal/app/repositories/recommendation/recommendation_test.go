package recommendation_test

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
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	testmocks "github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const waitFor = 5 * time.Second

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

type stubDiscovery struct {
	*mocks.Browser
}

func (stubDiscovery) Discover(
	context.Context,
	string,
	func(discovery.Result),
) (discovery.Outcome, error) {
	return discovery.Outcome{}, errors.New("recommendation never runs a query")
}

func defaultConfig() recommendation.Config {
	return recommendation.Config{
		Enabled:         true,
		RefreshInterval: "24h",
		CandidateBudget: 30,
		MinEntries:      1,
		Shelves: []recommendation.ShelfConfig{
			{
				ID:      "popular",
				Title:   "Popular",
				Limit:   24,
				Sources: []recommendation.SourceConfig{{Host: "github", Sort: "stars", MinStars: 500, MaxStars: 20000, PushedWithin: "90d"}},
			},
			{
				ID:      "fresh",
				Title:   "Recently updated",
				Limit:   24,
				Sources: []recommendation.SourceConfig{{Host: "github", Sort: "updated", MinStars: 100, PushedWithin: "30d"}},
			},
		},
	}
}

type fixture struct {
	db      *gorm.DB
	browser *mocks.Browser
	vault   *testmocks.Vault
	store   store.Store
	repo    recommendation.Recommendation
	done    chan struct{}
}

func newFixture(
	t *testing.T,
	cfg recommendation.Config,
	known discovery.KnownFn,
	answers ...mocks.BrowseAnswer,
) *fixture {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := store.New(db)
	require.NoError(t, err)

	f := &fixture{
		db:      db,
		browser: &mocks.Browser{Answers: answers},
		vault:   &testmocks.Vault{},
		store:   st,
		done:    make(chan struct{}, 16),
	}
	f.repo, err = recommendation.New(db, stubDiscovery{f.browser}, f.vault, known, cfg, recommendation.WithClock(func() time.Time { return now }))
	require.NoError(t, err)
	require.NoError(t, f.repo.OnHomeRefreshed(func(context.Context) { f.done <- struct{}{} }))
	t.Cleanup(func() { _ = f.repo.Shutdown(context.Background()) })
	return f
}

func (f *fixture) awaitRefresh(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(waitFor):
		t.Fatal("timed out waiting for a refresh")
	}
}

func indexRow(ns, ref string) vault.IndexRow {
	return vault.IndexRow{Namespace: domain.Namespace(ns), Ref: ref, Meta: vault.IndexMeta{Arrow: domain.ArrowMeta{Name: ns}}}
}

func ids(shelves []recommendation.Shelf) []string {
	out := make([]string, 0, len(shelves))
	for _, shelf := range shelves {
		out = append(out, shelf.ID)
	}
	return out
}

func TestNew_MissingDependencies_ReturnError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	disc := stubDiscovery{&mocks.Browser{}}

	_, noDisc := recommendation.New(db, nil, &testmocks.Vault{}, nil, defaultConfig())
	_, noVault := recommendation.New(db, disc, nil, nil, defaultConfig())
	_, noDB := recommendation.New(nil, disc, &testmocks.Vault{}, nil, defaultConfig())

	require.Error(t, noDisc)
	require.Error(t, noVault)
	require.Error(t, noDB)
}

func TestNew_BadRefreshInterval_ReturnsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	for _, interval := range []string{"", "soon", "0s", "-1h"} {
		cfg := defaultConfig()
		cfg.RefreshInterval = interval

		_, err := recommendation.New(db, stubDiscovery{&mocks.Browser{}}, &testmocks.Vault{}, nil, cfg)

		require.Error(t, err, interval)
	}
}

func TestNew_DisabledIgnoresTheRefreshInterval(t *testing.T) {
	cfg := defaultConfig()
	cfg.Enabled = false
	cfg.RefreshInterval = ""
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	_, err = recommendation.New(db, stubDiscovery{&mocks.Browser{}}, &testmocks.Vault{}, nil, cfg)

	require.NoError(t, err)
}

func TestDisabled_EveryOperationIsANoop(t *testing.T) {
	cfg := defaultConfig()
	cfg.Enabled = false
	f := newFixture(t, cfg, nil)
	ctx := context.Background()

	shelves, err := f.repo.Home(ctx)
	f.repo.Refresh(ctx)
	f.repo.Start(ctx)

	require.NoError(t, err)
	assert.NotNil(t, shelves)
	assert.Empty(t, shelves)
	assert.False(t, f.repo.Refreshing())
	assert.Zero(t, f.browser.Calls)
	assert.NoError(t, f.repo.Shutdown(ctx))
}

func TestOnHomeRefreshed_NilCallback_ReturnsError(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)

	require.Error(t, f.repo.OnHomeRefreshed(nil))
}

func TestHome_NeverRefreshed_ReturnsEveryShelfEmptyWithNoTimestamp(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)

	shelves, err := f.repo.Home(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"popular", "fresh"}, ids(shelves))
	assert.Equal(t, "Popular", shelves[0].Title)
	assert.True(t, shelves[0].RefreshedAt.IsZero())
	assert.Empty(t, shelves[0].Entries)
	assert.Zero(t, f.browser.Calls, "reading home never browses")
}

func TestHome_ReadsTheSnapshotAndAttachesVaultRows(t *testing.T) {
	known := func(_ context.Context, ns domain.Namespace) (bool, error) { return ns == "github.com/a/one", nil }
	f := newFixture(t, defaultConfig(), known)
	f.vault.SearchArrowsResult = []vault.IndexRow{
		indexRow("github.com/a/one", "v2"),
		indexRow("github.com/a/one-extra", "main"),
		indexRow("github.com/a/one", "v1"),
		indexRow("github.com/a/two", "main"),
	}
	at := now.Add(-time.Hour)
	require.NoError(t, f.store.Replace(context.Background(), "popular", at, []store.Entry{
		{Namespace: "github.com/a/one", Stars: 10, Source: "github.com"},
		{Namespace: "github.com/a/gone", Stars: 5, Source: "github.com"},
		{Namespace: "github.com/a/two", Stars: 7, Source: "gitlab.com"},
	}))

	shelves, err := f.repo.Home(context.Background())

	require.NoError(t, err)
	popular := shelves[0]
	assert.True(t, at.Equal(popular.RefreshedAt))
	require.Len(t, popular.Entries, 2, "an entry whose index row is gone is skipped")

	one := popular.Entries[0]
	assert.Equal(t, domain.Namespace("github.com/a/one"), one.Namespace)
	assert.Equal(t, 10, one.Stars)
	assert.Equal(t, "github.com", one.Source)
	assert.True(t, one.InCatalog)
	require.Len(t, one.Rows, 2, "rows of a namespace that merely shares a prefix are not its own")
	assert.Equal(t, "v2", one.Rows[0].Ref)

	two := popular.Entries[1]
	assert.False(t, two.InCatalog)
	assert.Equal(t, "gitlab.com", two.Source)
}

func TestHome_CatalogLookupFailureOrNoLookup_CostsTheFlagNotTheEntry(t *testing.T) {
	failing := func(context.Context, domain.Namespace) (bool, error) { return true, errors.New("boom") }

	for name, known := range map[string]discovery.KnownFn{"failing": failing, "absent": nil} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, defaultConfig(), known)
			f.vault.SearchArrowsResult = []vault.IndexRow{indexRow("github.com/a/one", "main")}
			require.NoError(t, f.store.Replace(context.Background(), "popular", now, []store.Entry{{Namespace: "github.com/a/one"}}))

			shelves, err := f.repo.Home(context.Background())

			require.NoError(t, err)
			require.Len(t, shelves[0].Entries, 1)
			assert.False(t, shelves[0].Entries[0].InCatalog)
		})
	}
}

func TestHome_VaultFailure_ReturnsError(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)
	f.vault.SearchArrowsErr = errors.New("index closed")
	require.NoError(t, f.store.Replace(context.Background(), "popular", now, []store.Entry{{Namespace: "github.com/a/one"}}))

	_, err := f.repo.Home(context.Background())

	require.Error(t, err)
}

func TestHome_StoreFailure_ReturnsError(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)
	require.NoError(t, f.db.Migrator().DropTable("recommendation_shelves"))

	_, err := f.repo.Home(context.Background())

	require.Error(t, err)
}

func TestHome_InvalidShelvesAreSkippedTheRestServed(t *testing.T) {
	good := recommendation.ShelfConfig{ID: "good", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", Sort: "stars", PushedWithin: "7d"}}}
	testCases := []struct {
		name string
		bad  recommendation.ShelfConfig
	}{
		{"no id", recommendation.ShelfConfig{Limit: 5, Sources: good.Sources}},
		{"duplicate id", recommendation.ShelfConfig{ID: "good", Limit: 5, Sources: good.Sources}},
		{"no limit", recommendation.ShelfConfig{ID: "bad", Sources: good.Sources}},
		{"no sources", recommendation.ShelfConfig{ID: "bad", Limit: 5}},
		{"no host", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Sort: "stars"}}}},
		{"unknown sort", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", Sort: "forks"}}}},
		{"negative stars", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", MinStars: -1}}}},
		{"negative max stars", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", MaxStars: -1}}}},
		{"max below min", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", MinStars: 500, MaxStars: 100}}}},
		{"bad window", recommendation.ShelfConfig{ID: "bad", Limit: 5, Sources: []recommendation.SourceConfig{{Host: "github", PushedWithin: "soon"}}}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig()
			cfg.Shelves = []recommendation.ShelfConfig{good, tc.bad}
			f := newFixture(t, cfg, nil)

			shelves, err := f.repo.Home(context.Background())

			require.NoError(t, err)
			assert.Equal(t, []string{"good"}, ids(shelves))
		})
	}
}

func TestRefresh_FillsTheSnapshotInTheBackgroundAndAnnouncesIt(t *testing.T) {
	cfg := defaultConfig()
	cfg.Shelves = cfg.Shelves[:1]
	f := newFixture(t, cfg, nil, mocks.BrowseAnswer{
		Emit:    []discovery.Result{mocks.Result("github.com/a/one", 4, "github.com")},
		Outcome: mocks.Outcome("github.com/a/one"),
	})
	f.vault.SearchArrowsResult = []vault.IndexRow{indexRow("github.com/a/one", "main")}

	f.repo.Refresh(context.Background())
	f.awaitRefresh(t)
	shelves, err := f.repo.Home(context.Background())

	require.NoError(t, err)
	require.Len(t, shelves[0].Entries, 1)
	assert.Equal(t, 4, shelves[0].Entries[0].Stars)
	assert.True(t, now.Equal(shelves[0].RefreshedAt))
	assert.Equal(t, 30, f.browser.Requests[0].Budget)
	assert.Equal(t, 100, f.browser.Requests[0].Sources[0].Limit)
	assert.Equal(t, 20000, f.browser.Requests[0].Sources[0].MaxStars)
	assert.Equal(t, 24, f.browser.Requests[0].Want)
	assert.Equal(t, 90*24*time.Hour, f.browser.Requests[0].Sources[0].PushedWithin)
}

func TestRefresh_ReportsRefreshingWhileRunningAndJoinsASecondTrigger(t *testing.T) {
	cfg := defaultConfig()
	cfg.Shelves = cfg.Shelves[:1]
	f := newFixture(t, cfg, nil, mocks.BrowseAnswer{Outcome: mocks.Outcome("github.com/a/one"), Emit: []discovery.Result{mocks.Result("github.com/a/one", 1, "github.com")}})
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	f.browser.OnBrowse = func(context.Context) {
		entered <- struct{}{}
		<-release
	}

	f.repo.Refresh(context.Background())
	<-entered
	f.repo.Refresh(context.Background())

	assert.True(t, f.repo.Refreshing())
	close(release)
	f.awaitRefresh(t)
	require.Eventually(t, func() bool { return !f.repo.Refreshing() }, waitFor, time.Millisecond)
	assert.Equal(t, 1, f.browser.Calls, "the second trigger joined the running refresh")
}

func TestRefresh_DropsSnapshotsOfShelvesTheConfigurationNoLongerHas(t *testing.T) {
	cfg := defaultConfig()
	cfg.Shelves = cfg.Shelves[:1]
	f := newFixture(t, cfg, nil, mocks.BrowseAnswer{Outcome: mocks.Outcome("github.com/a/one"), Emit: []discovery.Result{mocks.Result("github.com/a/one", 1, "github.com")}})
	require.NoError(t, f.store.Replace(context.Background(), "retired", now, []store.Entry{{Namespace: "github.com/a/old"}}))

	f.repo.Refresh(context.Background())
	f.awaitRefresh(t)

	snap, err := f.store.Snapshot(context.Background(), "retired")
	require.NoError(t, err)
	assert.Empty(t, snap.Entries)
}

func TestRefresh_StoreFailure_IsLoggedNotPropagated(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)
	require.NoError(t, f.db.Migrator().DropTable("recommendation_entries"))

	f.repo.Refresh(context.Background())

	require.Eventually(t, func() bool { return !f.repo.Refreshing() }, waitFor, time.Millisecond)
	assert.Zero(t, f.browser.Calls)
}

func TestStart_EmptySnapshot_RefreshesAtOnce(t *testing.T) {
	cfg := defaultConfig()
	cfg.Shelves = cfg.Shelves[:1]
	f := newFixture(t, cfg, nil, mocks.BrowseAnswer{Outcome: mocks.Outcome("github.com/a/one"), Emit: []discovery.Result{mocks.Result("github.com/a/one", 1, "github.com")}})

	f.repo.Start(context.Background())
	f.awaitRefresh(t)

	assert.Equal(t, 1, f.browser.Calls)
}

func TestDue(t *testing.T) {
	testCases := []struct {
		name  string
		write map[string]time.Time
		want  bool
	}{
		{name: "nothing ever refreshed", want: true},
		{name: "one shelf missing", write: map[string]time.Time{"popular": now.Add(-time.Hour)}, want: true},
		{name: "every shelf recent", write: map[string]time.Time{"popular": now.Add(-time.Hour), "fresh": now.Add(-2 * time.Hour)}, want: false},
		{name: "one shelf older than the interval", write: map[string]time.Time{"popular": now.Add(-time.Hour), "fresh": now.Add(-25 * time.Hour)}, want: true},
		{name: "exactly one interval old", write: map[string]time.Time{"popular": now.Add(-24 * time.Hour), "fresh": now}, want: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, defaultConfig(), nil)
			for id, at := range tc.write {
				require.NoError(t, f.store.Replace(context.Background(), id, at, nil))
			}

			assert.Equal(t, tc.want, recommendation.Due(f.repo, context.Background()))
		})
	}
}

func TestDue_UnreadableStore_CountsAsDue(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)
	require.NoError(t, f.db.Migrator().DropTable("recommendation_shelves"))

	assert.True(t, recommendation.Due(f.repo, context.Background()))
}

func TestShutdown_StopsTheSchedulerAndLaterRefreshesDoNothing(t *testing.T) {
	f := newFixture(t, defaultConfig(), nil)

	require.NoError(t, f.repo.Shutdown(context.Background()))
	f.repo.Refresh(context.Background())
	f.repo.Start(context.Background())

	assert.False(t, f.repo.Refreshing())
	assert.Zero(t, f.browser.Calls)
}
