package runtime_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// countTopic subscribes to topic and returns a counter of events seen, so a
// test can prove a no-op wrote nothing rather than inferring it from state.
func countTopic(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	topic string,
) *atomic.Int32 {
	t.Helper()
	var n atomic.Int32
	_, err := ax.Subscribe(asynx.Topic(topic), func(
		context.Context,
		asynxModels.Event[domainRuntime.ArrowRuntime],
	) {
		n.Add(1)
	})
	require.NoError(t, err)
	return &n
}

func seedState(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	state domain.ArrowState,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: state})
	require.NoError(t, err)
}

func TestSetVersionOutdated_ReadyAndDrifted_BecomesOutdated(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	require.NoError(t, runtime.SetVersionOutdated(ax)(context.Background(), ns, true))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
}

// The reverse direction: drift that resolved upstream without anyone running
// an update must give the arrow back, because Outdated is a state from which
// it cannot be run at all.
func TestSetVersionOutdated_OutdatedAndResolved_BecomesReady(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	set := runtime.SetVersionOutdated(ax)

	require.NoError(t, set(context.Background(), ns, true))
	require.NoError(t, set(context.Background(), ns, false))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// A dependency sync put this arrow at Outdated. A version check finding no
// drift is not entitled to clear that, and must not report a failure either.
func TestSetVersionOutdated_DepSyncOutdated_LeftAlone(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)

	rtRepo := newRepoWithAssembler(t, ax, successAssembler())
	require.NoError(t, rtRepo.MarkOutdated(
		context.Background(), ns, []domain.Namespace{"a/b/c@v1"}, nil,
	))
	ax.WaitPublish()

	require.NoError(t, runtime.SetVersionOutdated(ax)(context.Background(), ns, false))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
	assert.Equal(t, []domain.Namespace{"a/b/c@v1"}, got.PendingDepSync.AddedDeps)
}

// A version check runs for any namespace GetDetail is asked about, including
// ones nobody ever installed. It must not mint a runtime for them.
func TestSetVersionOutdated_NoAggregate_WritesNothing(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()

	require.NoError(t, runtime.SetVersionOutdated(ax)(context.Background(), ns, true))

	exists, err := ax.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

// A check that lands while the arrow is busy must leave it alone: Running has
// no transition to Outdated, and a version check is not an emergency.
func TestSetVersionOutdated_BusyArrow_LeftAlone(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	testCases := []struct {
		name  string
		state domain.ArrowState
	}{
		{"running", domain.ArrowStateRunning},
		{"installing", domain.ArrowStateInstalling},
		{"updating", domain.ArrowStateUpdating},
		{"uninstalling", domain.ArrowStateUninstalling},
		{"absent", domain.ArrowStateAbsent},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			seedState(t, ax, ns, tc.state)

			require.NoError(t, runtime.SetVersionOutdated(ax)(context.Background(), ns, true))

			got, err := ax.Get(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.state, got.State)
		})
	}
}

// The unaffected case, which is almost every check: no drift, already Ready —
// nothing is written at all.
func TestSetVersionOutdated_ReadyAndNoDrift_WritesNothing(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	fired := countTopic(t, ax, "runtime.outdated.*")
	cleared := countTopic(t, ax, "runtime.outdated_cleared.*")

	require.NoError(t, runtime.SetVersionOutdated(ax)(context.Background(), ns, false))
	ax.WaitPublish()

	assert.Equal(t, int32(0), fired.Load())
	assert.Equal(t, int32(0), cleared.Load())
}

// A reconfirmed drift answer must not re-announce itself on every check.
func TestSetVersionOutdated_AlreadyOutdated_WritesNothing(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	set := runtime.SetVersionOutdated(ax)
	require.NoError(t, set(context.Background(), ns, true))
	ax.WaitPublish()

	fired := countTopic(t, ax, "runtime.outdated.*")

	require.NoError(t, set(context.Background(), ns, true))
	ax.WaitPublish()

	assert.Equal(t, int32(0), fired.Load())
}

// The read cannot see everything the command validates, so the command stays
// the authoritative guard. An arrow whose state says Ready while an execution
// is genuinely in flight is refused there, and that refusal must surface as a
// state violation rather than as a raw asynx error.
func TestSetVersionOutdated_ExecutionInFlight_MapsToStateViolation(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	_, err := ax.Send(context.Background(), setRuntimeStateCmd{
		ns:    ns,
		state: domain.ArrowStateReady,
		exec:  &domainRuntime.Execution{ID: "exec-1"},
	})
	require.NoError(t, err)

	err = runtime.SetVersionOutdated(ax)(context.Background(), ns, true)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestSetVersionOutdated_StoreDown_ReturnsError(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	require.NoError(t, ax.Shutdown(context.Background()))

	err := runtime.SetVersionOutdated(ax)(context.Background(), ns, true)

	require.Error(t, err)
}
