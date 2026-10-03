package commands_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

func testPending() *domainRuntime.PendingActivation {
	return &domainRuntime.PendingActivation{
		Version:  "nightly-2",
		StagedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Path:     "/vault/core/quiver-new",
		Size:     42,
		Digest:   "abc",
	}
}

type emitter interface {
	EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime
}

func runningExecution(method string) *domainRuntime.Execution {
	return &domainRuntime.Execution{
		ID:     "exec-1",
		Method: method,
		Steps: []domainRuntime.StepProgress{
			{Index: 0, Status: domainRuntime.StepStatusPending, Step: domainStep.NewRunStep("s", "true", false, "", true)},
		},
	}
}

func TestEveryCommand_KeepsAStagedActivation(t *testing.T) {
	ns := testNs()
	ready := func() domainRuntime.ArrowRuntime {
		return domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: testPending()}
	}
	busy := func(method string, state domain.ArrowState) domainRuntime.ArrowRuntime {
		return domainRuntime.ArrowRuntime{Ref: ns, State: state, Execution: runningExecution(method), PendingActivation: testPending()}
	}

	testCases := []struct {
		name    string
		command emitter
		current domainRuntime.ArrowRuntime
	}{
		{"begin execution", commands.BeginExecution{Namespace: ns, ExecutionID: "e", Method: "_execute"}, ready()},
		{"begin stop", commands.BeginStop{Namespace: ns, ExecutionID: "e"}, busy(domain.MethodExecute, domain.ArrowStateRunning)},
		{"begin uninstall", commands.BeginUninstall{Namespace: ns, ExecutionID: "e"}, ready()},
		{"begin update", commands.BeginUpdate{Namespace: ns, ExecutionID: "e"}, ready()},
		{"advance step", commands.AdvanceStep{Namespace: ns, ExecutionID: "exec-1", StepIndex: 0, ToStatus: domainRuntime.StepStatusRunning}, busy(domain.MethodUpdate, domain.ArrowStateUpdating)},
		{"record pid", commands.RecordPID{Namespace: ns, ExecutionID: "exec-1", PID: 7}, busy(domain.MethodExecute, domain.ArrowStateRunning)},
		{"restart execution", commands.RestartExecution{Namespace: ns, ExecutionID: "exec-1"}, busy(domain.MethodUpdate, domain.ArrowStateUpdating)},
		{"end execution", commands.EndExecution{Namespace: ns, ExecutionID: "exec-1", Outcome: domainRuntime.ExecutionOutcomeSuccess}, busy(domain.MethodUpdate, domain.ArrowStateUpdating)},
		{"mark outdated", commands.MarkOutdated{Namespace: ns}, ready()},
		{"mark version outdated", commands.MarkVersionOutdated{Namespace: ns}, ready()},
		{"clear version outdated", commands.ClearVersionOutdated{Namespace: ns}, domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated, PendingActivation: testPending()}},
		{"record detached", commands.RecordDetached{Namespace: ns}, busy(domain.MethodExecute, domain.ArrowStateRunning)},
		{"record preinstalled", commands.RecordPreinstalled{Namespace: ns}, ready()},
		{"record self restored", commands.RecordSelfRestored{Namespace: ns}, busy(domain.MethodExecute, domain.ArrowStateRunning)},
		{"recover interrupted", commands.RecoverInterrupted{Namespace: ns}, busy(domain.MethodExecute, domain.ArrowStateRunning)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			next := tc.command.EmitEvent(&tc.current)

			assert.Equal(t, testPending(), next.PendingActivation)
		})
	}
}

func TestBeginInstall_DropsAStagedActivation(t *testing.T) {
	current := domainRuntime.ArrowRuntime{Ref: testNs(), State: domain.ArrowStateAbsent, PendingActivation: testPending()}

	next := commands.BeginInstall{Namespace: testNs(), ExecutionID: "e"}.EmitEvent(&current)

	assert.Nil(t, next.PendingActivation, "a fresh install starts from nothing staged")
}

func TestEndExecution_SuccessfulUninstall_DropsAStagedActivation(t *testing.T) {
	current := domainRuntime.ArrowRuntime{Ref: testNs(), State: domain.ArrowStateUninstalling, Execution: runningExecution(domain.MethodUninstall), PendingActivation: testPending()}

	next := commands.EndExecution{Namespace: testNs(), ExecutionID: "exec-1", Outcome: domainRuntime.ExecutionOutcomeSuccess}.EmitEvent(&current)

	assert.Nil(t, next.PendingActivation)
}

func TestEndExecution_FailedUninstall_KeepsAStagedActivation(t *testing.T) {
	current := domainRuntime.ArrowRuntime{Ref: testNs(), State: domain.ArrowStateUninstalling, Execution: runningExecution(domain.MethodUninstall), PendingActivation: testPending()}

	next := commands.EndExecution{Namespace: testNs(), ExecutionID: "exec-1", Outcome: domainRuntime.ExecutionOutcomeFailed}.EmitEvent(&current)

	assert.Equal(t, testPending(), next.PendingActivation)
}

func TestRecordPendingActivation_Validate(t *testing.T) {
	ns := testNs()
	testCases := []struct {
		name    string
		cmd     commands.RecordPendingActivation
		current *domainRuntime.ArrowRuntime
		wantErr bool
	}{
		{"ready row", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, false},
		{"outdated row", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated}, false},
		{"replaces an earlier one", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: testPending()}, false},
		{"no runtime", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, nil, true},
		{"empty ref", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{}, true},
		{"mid execution", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateRunning, Execution: runningExecution(domain.MethodExecute)}, true},
		{"no version", commands.RecordPendingActivation{Namespace: ns, Pending: domainRuntime.PendingActivation{Path: "/p"}}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, true},
		{"no path", commands.RecordPendingActivation{Namespace: ns, Pending: domainRuntime.PendingActivation{Version: "v"}}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, true},
		{"already activating", commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}, &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: &domainRuntime.PendingActivation{Version: "v", Path: "/p", Activating: true}}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Validate(tc.current)

			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, isValidationErr(err))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRecordPendingActivation_EmitsTheRecordAndKeepsTheRest(t *testing.T) {
	ns := testNs()
	ret := &domainRuntime.Return{ExecutionID: "old"}
	current := domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated, LastReturn: ret, PendingDepSync: &domainRuntime.DepSyncInfo{}}

	next := commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()}.EmitEvent(&current)

	assert.Equal(t, testPending(), next.PendingActivation)
	assert.Equal(t, domain.ArrowStateOutdated, next.State)
	assert.Same(t, ret, next.LastReturn)
	assert.Same(t, current.PendingDepSync, next.PendingDepSync)
	assert.Equal(t, "runtime.activation_staged."+ns.String(), commands.RecordPendingActivation{Namespace: ns}.EventName())
}

func TestMarkActivating_Validate(t *testing.T) {
	ns := testNs()
	staged := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: testPending()}
	already := &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: &domainRuntime.PendingActivation{Version: "v", Path: "/p", Activating: true}}

	require.NoError(t, commands.MarkActivating{Namespace: ns}.Validate(staged))
	for _, current := range []*domainRuntime.ArrowRuntime{nil, {}, {Ref: ns, State: domain.ArrowStateReady}, already} {
		err := commands.MarkActivating{Namespace: ns}.Validate(current)
		require.Error(t, err)
		assert.True(t, isValidationErr(err))
	}
}

func TestMarkActivating_EmitsAMarkedCopy(t *testing.T) {
	ns := testNs()
	current := domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: testPending()}

	next := commands.MarkActivating{Namespace: ns}.EmitEvent(&current)

	require.NotNil(t, next.PendingActivation)
	assert.True(t, next.PendingActivation.Activating)
	assert.Equal(t, "nightly-2", next.PendingActivation.Version)
	assert.False(t, current.PendingActivation.Activating, "the event does not mutate the state it was given")
	assert.Equal(t, "runtime.activation_marked."+ns.String(), commands.MarkActivating{Namespace: ns}.EventName())
}

func TestClearPendingActivation_Validate(t *testing.T) {
	ns := testNs()
	require.NoError(t, commands.ClearPendingActivation{Namespace: ns}.Validate(&domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: testPending()}))
	for _, current := range []*domainRuntime.ArrowRuntime{nil, {}, {Ref: ns, State: domain.ArrowStateReady}} {
		err := commands.ClearPendingActivation{Namespace: ns}.Validate(current)
		require.Error(t, err)
		assert.True(t, isValidationErr(err))
	}
}

func TestClearPendingActivation_EmitsNothingStagedAndKeepsTheRest(t *testing.T) {
	ns := testNs()
	ret := &domainRuntime.Return{ExecutionID: "old"}
	current := domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, LastReturn: ret, PendingActivation: testPending()}

	next := commands.ClearPendingActivation{Namespace: ns}.EmitEvent(&current)

	assert.Nil(t, next.PendingActivation)
	assert.Equal(t, domain.ArrowStateReady, next.State)
	assert.Same(t, ret, next.LastReturn)
	assert.Equal(t, "runtime.activation_cleared."+ns.String(), commands.ClearPendingActivation{Namespace: ns}.EventName())
}

func TestPendingActivationCommands_SurviveTheEventStore(t *testing.T) {
	ax := buildAsynx(t)
	ns := testNs()
	seedReadyRuntime(t, ax, ns)
	ctx := context.Background()

	_, err := ax.SendWait(ctx, commands.RecordPendingActivation{Namespace: ns, Pending: *testPending()})
	require.NoError(t, err)
	got, err := ax.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Equal(t, testPending(), got.PendingActivation)

	_, err = ax.SendWait(ctx, commands.MarkActivating{Namespace: ns})
	require.NoError(t, err)
	got, err = ax.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, got.PendingActivation.Activating)

	_, err = ax.SendWait(ctx, commands.ClearPendingActivation{Namespace: ns})
	require.NoError(t, err)
	got, err = ax.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.PendingActivation)
}
