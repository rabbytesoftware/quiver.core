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

func TestRecordPreinstalled_Metadata(t *testing.T) {
	ns := testNs()
	cmd := runtimecmds.RecordPreinstalled{Namespace: ns}

	assert.Equal(t, ns.String(), cmd.AggregateID())
	assert.Equal(t, "runtime.preinstalled."+ns.String(), cmd.EventName())
	assert.True(t, cmd.ShouldSnapshot())
}

// TestRecordPreinstalled_NoAggregate_CreatesReady is the case the whole feature
// turns on: a namespace with no runtime at all lands directly at Ready, with no
// execution ever having happened.
func TestRecordPreinstalled_NoAggregate_CreatesReady(t *testing.T) {
	ns := testNs()
	cmd := runtimecmds.RecordPreinstalled{Namespace: ns}

	require.NoError(t, cmd.Validate(nil))
	next := cmd.EmitEvent(nil)

	assert.Equal(t, ns, next.Ref)
	assert.Equal(t, domain.ArrowStateReady, next.State)
	assert.Nil(t, next.Execution)
	assert.Nil(t, next.LastReturn)
}

func TestRecordPreinstalled_EmptyRef_TreatedAsNoAggregate(t *testing.T) {
	cmd := runtimecmds.RecordPreinstalled{Namespace: testNs()}

	require.NoError(t, cmd.Validate(&domainRuntime.ArrowRuntime{}))
}

func TestRecordPreinstalled_FromAbsent_Ready(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateAbsent}
	cmd := runtimecmds.RecordPreinstalled{Namespace: ns}

	require.NoError(t, cmd.Validate(current))
	assert.Equal(t, domain.ArrowStateReady, cmd.EmitEvent(current).State)
}

// TestRecordPreinstalled_FromReady_IsIdempotent: Add writes the runtime before
// the catalog row, so an Add that fails afterwards leaves a Ready runtime the
// next Add has to be able to converge on rather than trip over.
func TestRecordPreinstalled_FromReady_IsIdempotent(t *testing.T) {
	ns := testNs()
	ret := &domainRuntime.Return{Method: domain.MethodInstall}
	current := &domainRuntime.ArrowRuntime{
		Ref:            ns,
		State:          domain.ArrowStateReady,
		LastReturn:     ret,
		PendingDepSync: &domainRuntime.DepSyncInfo{AddedDeps: []domain.Namespace{"a/b/c@v1"}},
	}
	cmd := runtimecmds.RecordPreinstalled{Namespace: ns}

	require.NoError(t, cmd.Validate(current))
	next := cmd.EmitEvent(current)

	assert.Equal(t, domain.ArrowStateReady, next.State)
	assert.Equal(t, ret, next.LastReturn, "an existing return history survives")
	require.NotNil(t, next.PendingDepSync)
	assert.Equal(t, []domain.Namespace{"a/b/c@v1"}, next.PendingDepSync.AddedDeps)
}

func TestRecordPreinstalled_Validate_RejectsRunningExecution(t *testing.T) {
	ns := testNs()
	current := &domainRuntime.ArrowRuntime{
		Ref:       ns,
		State:     domain.ArrowStateAbsent,
		Execution: &domainRuntime.Execution{ID: "exec-1"},
	}

	err := runtimecmds.RecordPreinstalled{Namespace: ns}.Validate(current)

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}

func TestRecordPreinstalled_Validate_RejectsOtherStates(t *testing.T) {
	ns := testNs()
	testCases := []struct {
		name  string
		state domain.ArrowState
	}{
		{"installing", domain.ArrowStateInstalling},
		{"running", domain.ArrowStateRunning},
		{"uninstalling", domain.ArrowStateUninstalling},
		{"updating", domain.ArrowStateUpdating},
		{"detached", domain.ArrowStateDetached},
		{"outdated", domain.ArrowStateOutdated},
		{"removed", domain.ArrowStateRemoved},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := runtimecmds.RecordPreinstalled{Namespace: ns}.Validate(
				&domainRuntime.ArrowRuntime{Ref: ns, State: tc.state},
			)

			require.Error(t, err)
			assert.True(t, isValidationErr(err))
		})
	}
}

func TestRecordPreinstalled_ThroughAsynx_CreatesReadyAggregate(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.SendWait(context.Background(), runtimecmds.RecordPreinstalled{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.Equal(t, ns, got.Ref)
	assert.Nil(t, got.Execution)
}

func TestRecordPreinstalled_ThroughAsynx_TwiceStaysReady(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()

	_, err := ax.SendWait(context.Background(), runtimecmds.RecordPreinstalled{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.SendWait(context.Background(), runtimecmds.RecordPreinstalled{Namespace: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// TestRecordPreinstalled_ThroughAsynx_MidInstall_Rejected: a detection must
// never be able to overwrite an install that is actually in flight.
func TestRecordPreinstalled_ThroughAsynx_MidInstall_Rejected(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedRuntime(t, ax, ns, nil)

	_, err := ax.SendWait(context.Background(), runtimecmds.RecordPreinstalled{Namespace: ns})

	require.Error(t, err)
	assert.True(t, isValidationErr(err))
}
