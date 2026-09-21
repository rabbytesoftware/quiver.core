package commands_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestMarkVersionOutdated_Metadata(t *testing.T) {
	ns := testNs()
	cmd := runtimecmds.MarkVersionOutdated{Namespace: ns}

	assert.Equal(t, ns.String(), cmd.AggregateID())
	assert.Equal(t, "runtime.outdated."+ns.String(), cmd.EventName(),
		"it must share MarkOutdated's topic so the already-wired hub broadcast carries it")
	assert.True(t, cmd.ShouldSnapshot())
}

func TestMarkVersionOutdated_FromReady_SetsOutdated(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}
	cmd := runtimecmds.MarkVersionOutdated{Namespace: ns}

	require.NoError(t, cmd.Validate(current))
	next := cmd.EmitEvent(current)

	assert.Equal(t, ns, next.Ref)
	assert.Equal(t, domain.ArrowStateOutdated, next.State)
}

// The whole reason this command exists rather than MarkOutdated: a newer
// release existing is not a dependency-graph change, so it must never invent a
// PendingDepSync.
func TestMarkVersionOutdated_NeverInventsPendingDepSync(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}

	next := runtimecmds.MarkVersionOutdated{Namespace: ns}.EmitEvent(current)

	assert.Nil(t, next.PendingDepSync,
		"version drift must not read downstream as a dependency sync that changed nothing")
}

// The other half of keeping the two concepts apart: a dep sync that really is
// pending survives a version check firing on the same arrow.
func TestMarkVersionOutdated_CarriesPendingDepSyncThrough(t *testing.T) {
	ns := testNs()
	sync := &domainRuntime.DepSyncInfo{AddedDeps: []domain.Namespace{"a/b/c@v1"}}
	ret := &domainRuntime.Return{Method: domain.MethodInstall}
	current := &domainRuntime.ArrowRuntime{
		Ref:            ns,
		State:          domain.ArrowStateReady,
		LastReturn:     ret,
		PendingDepSync: sync,
	}

	next := runtimecmds.MarkVersionOutdated{Namespace: ns}.EmitEvent(current)

	assert.Equal(t, sync, next.PendingDepSync, "a real pending dep sync must not be destroyed")
	assert.Equal(t, ret, next.LastReturn, "an existing return history survives")
}

// Re-marking must converge, not trip: a later check that finds a different
// recommended ref sends this again while the runtime already sits at Outdated.
func TestMarkVersionOutdated_FromOutdated_IsIdempotent(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated}
	cmd := runtimecmds.MarkVersionOutdated{Namespace: ns}

	require.NoError(t, cmd.Validate(current))
	assert.Equal(t, domain.ArrowStateOutdated, cmd.EmitEvent(current).State)
}

// Unlike MarkOutdated, this must refuse to conjure a runtime aggregate out of
// nothing: a version check runs for any namespace GetDetail is asked about,
// including ones nobody ever installed.
func TestMarkVersionOutdated_Validate_RejectsNoAggregate(t *testing.T) {
	cmd := runtimecmds.MarkVersionOutdated{Namespace: testNs()}

	require.Error(t, cmd.Validate(nil))
	assert.True(t, isValidationErr(cmd.Validate(nil)))

	err := cmd.Validate(&domainRuntime.ArrowRuntime{})
	require.Error(t, err, "an empty Ref is the same as no aggregate")
	assert.True(t, isValidationErr(err))
}

// Validate rejects nil, so asynx never reaches EmitEvent with one. It still
// must not panic if anything ever calls it directly.
func TestMarkVersionOutdated_EmitEvent_NilCurrent_NoPanic(t *testing.T) {
	ns := testNs()

	next := runtimecmds.MarkVersionOutdated{Namespace: ns}.EmitEvent(nil)

	assert.Equal(t, domain.ArrowStateOutdated, next.State)
	assert.Nil(t, next.PendingDepSync)
	assert.Nil(t, next.LastReturn)
}

func TestMarkVersionOutdated_Validate_RejectsInFlightExecution(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{
		Ref:       ns,
		State:     domain.ArrowStateReady,
		Execution: &domainRuntime.Execution{ID: "exec-1"},
	}

	err := runtimecmds.MarkVersionOutdated{Namespace: ns}.Validate(current)

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestMarkVersionOutdated_Validate_RejectsOtherStates(t *testing.T) {
	ns := testNs()
	testCases := []struct {
		name  string
		state domain.ArrowState
	}{
		{"absent", domain.ArrowStateAbsent},
		{"installing", domain.ArrowStateInstalling},
		{"running", domain.ArrowStateRunning},
		{"stopping", domain.ArrowStateStopping},
		{"draining", domain.ArrowStateDraining},
		{"detached", domain.ArrowStateDetached},
		{"uninstalling", domain.ArrowStateUninstalling},
		{"updating", domain.ArrowStateUpdating},
		{"removed", domain.ArrowStateRemoved},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runtimecmds.MarkVersionOutdated{Namespace: ns}.Validate(
				&domainRuntime.ArrowRuntime{Ref: ns, State: tc.state},
			)

			require.Error(t, err)
			assert.True(t, isValidationErr(err))
		})
	}
}

func TestMarkVersionOutdated_ThroughAsynx_ReadyBecomesOutdated(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.SendWait(context.Background(), runtimecmds.MarkVersionOutdated{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	assert.Nil(t, got.PendingDepSync)
}

func TestMarkVersionOutdated_ThroughAsynx_NoAggregate_Rejected(t *testing.T) {
	ax := buildAsynx(t)

	_, err := ax.SendWait(context.Background(), runtimecmds.MarkVersionOutdated{Namespace: testNs()})

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}
