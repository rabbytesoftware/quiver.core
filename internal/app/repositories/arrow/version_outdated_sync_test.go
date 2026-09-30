package arrow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	runtimeRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ─── runVersionCheck → ArrowRuntime.State ────────────────────────────────────
//
// The badge the frontend renders reads ArrowRuntime.State, not Arrow.Available.
// These cover the aggregate the version check never used to touch.

// driftingCatalog builds a catalog whose drift answer is fixed, wired to a real
// runtime aggregate through the same closure the container wires in production.
func driftingCatalog(
	t *testing.T,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	available *domain.Available,
) arrowRepo.Arrow {
	t.Helper()
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			return available, true
		},
	}
	return arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(runtimeRepo.SetVersionOutdated(axRuntime)))
}

func ahead() *domain.Available {
	return &domain.Available{Ref: "v2.0.0", Commit: "c2"}
}

// seedCatalogued adds ns to the arrow aggregate and returns the stored arrow,
// which is what runVersionCheck is handed.
func seedCatalogued(
	t *testing.T,
	axArrow asynx.Asynx[domain.Arrow],
	ns domain.Namespace,
) domain.Arrow {
	t.Helper()
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	return arrow
}

func runtimeState(
	t *testing.T,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) domain.ArrowState {
	t.Helper()
	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	return got.State
}

// The bug this task exists for: drift was recorded on the catalog aggregate and
// the runtime aggregate — the one the badge reads — never moved.
func TestRunVersionCheck_DriftFound_TransitionsRuntimeToOutdated(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	cat := driftingCatalog(t, axArrow, axRuntime, ahead())
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns),
		"the badge reads State, so State is what has to move")

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ahead(), got.Available, "the catalog record must still be written too")
}

// The unaffected case: an arrow with no drift sees no state change at all.
func TestRunVersionCheck_NoDrift_LeavesRuntimeReady(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	cat := driftingCatalog(t, axArrow, axRuntime, nil)
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns))
}

// The reverse transition: upstream yanked the newer release, so the drift is
// over without anyone having run an update. Outdated is a state the arrow
// cannot be run from, so it has to be given back.
func TestRunVersionCheck_DriftResolved_TransitionsRuntimeBackToReady(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	arrowRepo.RunVersionCheckForTest(
		driftingCatalog(t, axArrow, axRuntime, ahead()), context.Background(), arrow)
	require.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))

	arrowRepo.RunVersionCheckForTest(
		driftingCatalog(t, axArrow, axRuntime, nil), context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Available, "the catalog record reverses too, as it always did")
}

// The case that would have made this fix dead on arrival. Every arrow already
// carrying Available from before the fix takes runVersionCheck's
// "answer unchanged" early return on every later check, so a runtime write
// placed after it would never run for exactly the installs that exposed the
// bug. The reconcile is therefore unconditional.
func TestRunVersionCheck_AnswerUnchanged_StillReconcilesRuntime(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	// Exactly the field state a pre-fix install is sitting in: the catalog
	// already says outdated, the runtime still says ready.
	_, err := axArrow.SendWait(context.Background(), arrowcmds.RecordAvailable{
		Namespace: ns,
		Available: ahead(),
	})
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns))

	cat := driftingCatalog(t, axArrow, axRuntime, ahead())
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns),
		"a check that reconfirms the catalog's answer must still repair the runtime")
}

// A namespace nobody ever installed has no runtime aggregate, and a version
// check must not create one.
func TestRunVersionCheck_NeverInstalled_CreatesNoRuntime(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)

	cat := driftingCatalog(t, axArrow, axRuntime, ahead())
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	exists, err := axRuntime.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ahead(), got.Available, "the catalog record is still worth writing")
}

// A container built without the sync behaves exactly as it always has.
func TestRunVersionCheck_SyncNotWired_StillRecordsOnCatalog(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)

	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			return ahead(), true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ahead(), got.Available)
}

// A sync that fails must not cost the catalog its record: the reconcile is
// unconditional, so the next check retries it.
func TestRunVersionCheck_SyncFails_CatalogRecordStillWritten(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)

	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			return ahead(), true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(context.Context, domain.Namespace, bool) error {
			return errors.New("runtime store down")
		}))

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ahead(), got.Available)
}

// ok=false aborts before either aggregate is touched — a resolution failure is
// never a trustworthy "no drift" either, and must not clear a real one.
func TestRunVersionCheck_ResolutionFailed_DoesNotTouchRuntime(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	arrowRepo.RunVersionCheckForTest(
		driftingCatalog(t, axArrow, axRuntime, ahead()), context.Background(), arrow)
	require.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))

	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			return nil, false
		},
	}
	failing := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(runtimeRepo.SetVersionOutdated(axRuntime)))
	arrowRepo.RunVersionCheckForTest(failing, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))
}

// ─── CheckInstalledVersions ──────────────────────────────────────────────────

func TestCheckInstalledVersions(t *testing.T) {
	installed := domain.Namespace("github.com/user/installed@stable")
	claimedElsewhere := domain.Namespace("github.com/user/recent@stable")
	catalogued := domain.Namespace("github.com/user/catalogued@stable")
	at := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	views := []models.ArrowView{
		{Versions: []models.VersionView{
			{Namespace: installed, Metadata: domain.Arrow{InstalledAt: at}},
			{Namespace: catalogued},
		}},
		{Versions: []models.VersionView{{Namespace: claimedElsewhere, Metadata: domain.Arrow{InstalledAt: at}}}},
	}

	testCases := []struct {
		name    string
		listErr error
		cancel  bool
		want    []domain.Namespace
	}{
		{name: "every installed row whose claim is won", want: []domain.Namespace{installed}},
		{name: "the catalog cannot be listed", listErr: errors.New("db closed"), want: nil},
		{name: "a stopped sweep checks nothing", cancel: true, want: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			axArrow := newTestAsynxArrow(t)
			for _, ns := range []domain.Namespace{installed, claimedElsewhere, catalogued} {
				seedCatalogued(t, axArrow, ns)
			}
			var checked []domain.Namespace
			r := &arrowStoreMocks.MockCQRS{
				ListFn: func(context.Context, *bool) ([]models.ArrowView, error) { return views, tc.listErr },
				NeedsVersionCheckFn: func(_ context.Context, ns domain.Namespace, _ time.Time) (bool, error) {
					return ns != claimedElsewhere, nil
				},
				CheckDriftFn: func(_ context.Context, a domain.Arrow) (*domain.Available, bool) {
					checked = append(checked, a.Namespace)
					return nil, true
				},
			}
			cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}

			cat.CheckInstalledVersions(ctx)

			assert.Equal(t, tc.want, checked)
		})
	}
}

func TestCheckInstalledVersions_RowGoneSinceListing_IsSkipped(t *testing.T) {
	gone := domain.Namespace("github.com/user/gone@stable")
	checks := 0
	r := &arrowStoreMocks.MockCQRS{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{Versions: []models.VersionView{{Namespace: gone, Metadata: domain.Arrow{InstalledAt: time.Now()}}}}}, nil
		},
		NeedsVersionCheckFn: func(context.Context, domain.Namespace, time.Time) (bool, error) { return true, nil },
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			checks++
			return nil, true
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	cat.CheckInstalledVersions(context.Background())

	assert.Zero(t, checks)
}

// While an update of the row settles, the row still names the target it is
// about to stamp; a check landing then records what it found but leaves the
// badge to the update, which re-derives it once its commit landed.
func TestRunVersionCheck_RowSettling_LeavesTheBadgeToTheUpdate(t *testing.T) {
	testCases := []struct {
		name     string
		settling bool
		want     domain.ArrowState
	}{
		{name: "an update is settling", settling: true, want: domain.ArrowStateReady},
		{name: "no update is settling", settling: false, want: domain.ArrowStateOutdated},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			axArrow := newTestAsynxArrow(t)
			axRuntime := newTestAsynxRuntime(t)
			ns := testNs()
			arrow := seedCatalogued(t, axArrow, ns)
			require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))
			cat := driftingCatalog(t, axArrow, axRuntime, ahead())
			cat.HoldBadgeWhile(func(got domain.Namespace) bool { return tc.settling && got == ns })

			arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

			assert.Equal(t, tc.want, runtimeState(t, axRuntime, ns))
			got, err := axArrow.Get(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, ahead(), got.Available, "the finding is recorded either way")
		})
	}
}
