package runtime_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func catalogReturning(
	arrow *domain.Arrow,
	err error,
) runtime.GetArrowFn {
	return func(context.Context, domain.Namespace) (*domain.Arrow, error) {
		return arrow, err
	}
}

// The badge-window fix in one call: the runtime says Ready, the catalog still
// says a newer release exists, and the reconcile makes the state agree with the
// fact it projects.
func TestReconcileVersionBadge_CatalogStillDrifted_BecomesOutdated(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	reconcile := runtime.ReconcileVersionBadge(
		catalogReturning(&domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "v1.1.0", Commit: "abc"}}, nil),
		ax,
	)
	require.NoError(t, reconcile(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
}

// The ordinary case writes nothing at all: a Ready arrow with no drift is
// already where the catalog says it should be.
func TestReconcileVersionBadge_NoDrift_WritesNothing(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	outdatedEvents := countTopic(t, ax, "runtime.outdated.*")

	reconcile := runtime.ReconcileVersionBadge(
		catalogReturning(&domain.Arrow{Namespace: ns}, nil),
		ax,
	)
	require.NoError(t, reconcile(context.Background(), ns))
	ax.WaitPublish()

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.Zero(t, outdatedEvents.Load())
}

// An arrow whose catalog row is gone — forgotten while it ran — has no answer
// to project. That is an error, not a quiet "no drift": treating it as the
// latter would clear a badge nobody asked to clear.
func TestReconcileVersionBadge_CatalogReadFails_ReturnsError(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	reconcile := runtime.ReconcileVersionBadge(catalogReturning(nil, assert.AnError), ax)

	err := reconcile(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// A catalog that answers with no arrow and no error has said nothing; there is
// nothing to reconcile against and nothing to report.
func TestReconcileVersionBadge_NoArrowNoError_NoOp(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	reconcile := runtime.ReconcileVersionBadge(catalogReturning(nil, nil), ax)
	require.NoError(t, reconcile(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// A namespace nothing ever installed has no runtime aggregate. Looking at a
// drifted catalog row must not grow one, exactly as a version check must not.
func TestReconcileVersionBadge_NoRuntimeAggregate_NoOp(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()

	reconcile := runtime.ReconcileVersionBadge(
		catalogReturning(&domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "v1.1.0", Commit: "abc"}}, nil),
		ax,
	)
	require.NoError(t, reconcile(context.Background(), ns))

	exists, err := ax.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

// The ordering guard: the reconcile reads the catalog at the moment it runs,
// never a value captured earlier. An advance that commits between two
// reconciles (Available cleared) must win, whichever of the post-update
// handler and the drain's reconcile lands first.
func TestReconcileVersionBadge_ReadsAvailableAtRunTime(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	var reads atomic.Int32
	getArrow := func(context.Context, domain.Namespace) (*domain.Arrow, error) {
		if reads.Add(1) == 1 {
			return &domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "v1.1.0", Commit: "abc"}}, nil
		}
		return &domain.Arrow{Namespace: ns}, nil
	}
	reconcile := runtime.ReconcileVersionBadge(getArrow, ax)

	require.NoError(t, reconcile(context.Background(), ns))
	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)

	require.NoError(t, reconcile(context.Background(), ns))
	got, err = ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.Equal(t, int32(2), reads.Load())
}

// A reconcile that ran before the advance committed left the badge up; the
// advance's own ClearVersionBadge takes it down, so the two orders converge.
func TestReconcileVersionBadge_StaleReconcileThenClear_ConvergesReady(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	repo := newRepoWithAssembler(t, ax, successAssembler())

	stale := runtime.ReconcileVersionBadge(
		catalogReturning(&domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "v1.1.0"}}, nil),
		ax,
	)
	require.NoError(t, stale(context.Background(), ns))
	require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}
