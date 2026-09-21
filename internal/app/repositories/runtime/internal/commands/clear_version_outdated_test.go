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

func TestClearVersionOutdated_Metadata(t *testing.T) {
	ns := testNs()
	cmd := runtimecmds.ClearVersionOutdated{Namespace: ns}

	assert.Equal(t, ns.String(), cmd.AggregateID())
	assert.Equal(t, "runtime.outdated_cleared."+ns.String(), cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot())
}

func TestClearVersionOutdated_FromOutdated_SetsReady(t *testing.T) {
	ns := testNs()
	ret := &domainRuntime.Return{Method: domain.MethodInstall}
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated, LastReturn: ret}
	cmd := runtimecmds.ClearVersionOutdated{Namespace: ns}

	require.NoError(t, cmd.Validate(current))
	next := cmd.EmitEvent(current)

	assert.Equal(t, ns, next.Ref)
	assert.Equal(t, domain.ArrowStateReady, next.State)
	assert.Equal(t, ret, next.LastReturn, "an existing return history survives")
	assert.Nil(t, next.Execution)
	assert.Nil(t, next.PendingDepSync)
}

// The guard rail that makes the reverse transition safe at all: an Outdated a
// dependency sync put there is not a version check's to clear. The deleted
// ClearOutdated discarded PendingDepSync; this refuses instead.
func TestClearVersionOutdated_Validate_RejectsPendingDepSync(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{
		Ref:            ns,
		State:          domain.ArrowStateOutdated,
		PendingDepSync: &domainRuntime.DepSyncInfo{AddedDeps: []domain.Namespace{"a/b/c@v1"}},
	}

	err := runtimecmds.ClearVersionOutdated{Namespace: ns}.Validate(current)

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

// An empty-but-present DepSyncInfo is still a dependency sync as far as
// syncDeps' own nil test is concerned, so it must be refused too.
func TestClearVersionOutdated_Validate_RejectsEmptyPendingDepSync(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{
		Ref:            ns,
		State:          domain.ArrowStateOutdated,
		PendingDepSync: &domainRuntime.DepSyncInfo{},
	}

	err := runtimecmds.ClearVersionOutdated{Namespace: ns}.Validate(current)

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestClearVersionOutdated_Validate_RejectsNoAggregate(t *testing.T) {
	cmd := runtimecmds.ClearVersionOutdated{Namespace: testNs()}

	require.Error(t, cmd.Validate(nil))
	assert.True(t, isValidationErr(cmd.Validate(nil)))

	err := cmd.Validate(&domainRuntime.ArrowRuntime{})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestClearVersionOutdated_Validate_RejectsInFlightExecution(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{
		Ref:       ns,
		State:     domain.ArrowStateOutdated,
		Execution: &domainRuntime.Execution{ID: "exec-1"},
	}

	err := runtimecmds.ClearVersionOutdated{Namespace: ns}.Validate(current)

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestClearVersionOutdated_Validate_RejectsOtherStates(t *testing.T) {
	ns := testNs()
	testCases := []struct {
		name  string
		state domain.ArrowState
	}{
		{"ready", domain.ArrowStateReady},
		{"absent", domain.ArrowStateAbsent},
		{"installing", domain.ArrowStateInstalling},
		{"running", domain.ArrowStateRunning},
		{"updating", domain.ArrowStateUpdating},
		{"uninstalling", domain.ArrowStateUninstalling},
		{"detached", domain.ArrowStateDetached},
		{"removed", domain.ArrowStateRemoved},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runtimecmds.ClearVersionOutdated{Namespace: ns}.Validate(
				&domainRuntime.ArrowRuntime{Ref: ns, State: tc.state},
			)

			require.Error(t, err)
			assert.True(t, isValidationErr(err))
		})
	}
}

func TestClearVersionOutdated_ThroughAsynx_OutdatedBecomesReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.SendWait(context.Background(), runtimecmds.MarkVersionOutdated{Namespace: ns})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), runtimecmds.ClearVersionOutdated{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// Proven end to end through the aggregate rather than by inspecting Validate:
// a dependency-sync Outdated survives a version check deciding drift is over.
func TestClearVersionOutdated_ThroughAsynx_DepSyncOutdated_Refused(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.SendWait(context.Background(), runtimecmds.MarkOutdated{
		Namespace: ns,
		AddedDeps: []domain.Namespace{"a/b/c@v1"},
	})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), runtimecmds.ClearVersionOutdated{Namespace: ns})
	require.Error(t, err)
	assert.True(t, isValidationErr(err))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
	assert.Equal(t, []domain.Namespace{"a/b/c@v1"}, got.PendingDepSync.AddedDeps)
}
