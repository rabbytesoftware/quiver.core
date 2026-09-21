package arrow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	runtimeRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ─── runVersionCheck → ArrowRuntime.State ────────────────────────────────────
//
// The badge the frontend renders reads ArrowRuntime.State, not Arrow.Outdated.
// These cover the aggregate the version check never used to touch.

// driftingCatalog builds a catalog whose drift answer is fixed, wired to a real
// runtime aggregate through the same closure the container wires in production.
func driftingCatalog(
	t *testing.T,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	outdated bool,
	recommendedRef string,
) arrowRepo.Arrow {
	t.Helper()
	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return outdated, recommendedRef, true
		},
	}
	return arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(runtimeRepo.SetVersionOutdated(axRuntime)))
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

	cat := driftingCatalog(t, axArrow, axRuntime, true, "v2.0.0")
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns),
		"the badge reads State, so State is what has to move")

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.Outdated, "the catalog record must still be written too")
	assert.Equal(t, "v2.0.0", got.RecommendedRef)
}

// The unaffected case: an arrow with no drift sees no state change at all.
func TestRunVersionCheck_NoDrift_LeavesRuntimeReady(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(context.Background(), ns))

	cat := driftingCatalog(t, axArrow, axRuntime, false, "")
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
		driftingCatalog(t, axArrow, axRuntime, true, "v2.0.0"), context.Background(), arrow)
	require.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))

	arrowRepo.RunVersionCheckForTest(
		driftingCatalog(t, axArrow, axRuntime, false, ""), context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.Outdated, "the catalog record reverses too, as it always did")
}

// The case that would have made this fix dead on arrival. Every arrow already
// carrying Outdated=true from before the fix takes runVersionCheck's
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
	_, err := axArrow.SendWait(context.Background(), arrowcmds.RecordVersionCheck{
		Namespace:      ns,
		Outdated:       true,
		RecommendedRef: "v2.0.0",
	})
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns))

	cat := driftingCatalog(t, axArrow, axRuntime, true, "v2.0.0")
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

	cat := driftingCatalog(t, axArrow, axRuntime, true, "v2.0.0")
	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	exists, err := axRuntime.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.Outdated, "the catalog record is still worth writing")
}

// A container built without the sync behaves exactly as it always has.
func TestRunVersionCheck_SyncNotWired_StillRecordsOnCatalog(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.Outdated)
}

// A sync that fails must not cost the catalog its record: the reconcile is
// unconditional, so the next check retries it.
func TestRunVersionCheck_SyncFails_CatalogRecordStillWritten(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := seedCatalogued(t, axArrow, ns)

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(context.Context, domain.Namespace, bool) error {
			return errors.New("runtime store down")
		}))

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.Outdated)
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
		driftingCatalog(t, axArrow, axRuntime, true, "v2.0.0"), context.Background(), arrow)
	require.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return false, "", false
		},
	}
	failing := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(runtimeRepo.SetVersionOutdated(axRuntime)))
	arrowRepo.RunVersionCheckForTest(failing, context.Background(), arrow)

	assert.Equal(t, domain.ArrowStateOutdated, runtimeState(t, axRuntime, ns))
}
