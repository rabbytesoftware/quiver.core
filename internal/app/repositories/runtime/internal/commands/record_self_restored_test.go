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

func TestRecordSelfRestored_FromDetached_RestoresExecution(t *testing.T) {
	ns := domain.Namespace("test/user/repo@v1")
	current := &domainRuntime.ArrowRuntime{
		Ref:   ns,
		State: domain.ArrowStateDetached,
	}
	cmd := runtimecmds.RecordSelfRestored{
		Namespace: ns,
		Execution: &domainRuntime.Execution{
			ID:      "exec-1",
			Method:  domain.MethodExecute,
			PID:     4242,
			WorkDir: "/tmp/work",
		},
	}

	require.NoError(t, cmd.Validate(current))
	next := cmd.EmitEvent(current)

	assert.Equal(t, domain.ArrowStateRunning, next.State)
	require.NotNil(t, next.Execution)
	assert.Equal(t, 4242, next.Execution.PID)
}

// TestRecordSelfRestored_FromRunning_RestoresExecution covers the state that
// recoverRunning actually observes: RecoverTransients only calls recoverRunning
// when the stored state is already Running (its switch never routes Detached
// there), so RecordSelfRestored's one real caller always validates against a
// current state of Running, not Detached — this must succeed too.
func TestRecordSelfRestored_FromRunning_RestoresExecution(t *testing.T) {
	ns := domain.Namespace("test/user/repo@v1")
	current := &domainRuntime.ArrowRuntime{
		Ref:   ns,
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			ID:  "exec-1",
			PID: 4242,
		},
	}
	cmd := runtimecmds.RecordSelfRestored{
		Namespace: ns,
		Execution: current.Execution,
	}

	require.NoError(t, cmd.Validate(current))
	next := cmd.EmitEvent(current)

	assert.Equal(t, domain.ArrowStateRunning, next.State)
	require.NotNil(t, next.Execution)
	assert.Equal(t, 4242, next.Execution.PID)
}

func TestRecordSelfRestored_Validate_RejectsNonDetached(t *testing.T) {
	ns := domain.Namespace("test/user/repo@v1")
	current := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}
	cmd := runtimecmds.RecordSelfRestored{Namespace: ns}

	err := cmd.Validate(current)

	require.Error(t, err)
}

func TestRecordSelfRestored_Validate_RejectsNilCurrent(t *testing.T) {
	cmd := runtimecmds.RecordSelfRestored{Namespace: "test/user/repo@v1"}

	err := cmd.Validate(nil)

	require.Error(t, err)
}

// ─── RecordSelfRestored through a real Asynx instance ───────────────────────
//
// buildAsynx/testNs/seedReadyRuntime are shared helpers defined in
// commands_test.go, in this same package.

func TestRecordSelfRestored_FromRunning_ThroughAsynx_SetsRunning(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.Send(context.Background(), runtimecmds.BeginExecution{
		Namespace:   ns,
		Method:      domain.MethodExecute,
		AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
	})
	require.NoError(t, err)

	_, err = ax.SendWait(context.Background(), runtimecmds.RecordSelfRestored{
		Namespace: ns,
		Execution: &domainRuntime.Execution{ID: "exec-1", PID: 777},
	})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateRunning, got.State)
	require.NotNil(t, got.Execution)
	assert.Equal(t, 777, got.Execution.PID)
}

func TestRecordSelfRestored_ThroughAsynx_NotFromDetachedOrRunning_Fails(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)

	_, err := ax.SendWait(context.Background(), runtimecmds.RecordSelfRestored{Namespace: ns})

	require.Error(t, err)
}
