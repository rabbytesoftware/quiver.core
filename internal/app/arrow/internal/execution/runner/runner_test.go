package runner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

func buildAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

func buildAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

// testRunner builds a runnerService for whitebox tests.
func testRunner(
	t *testing.T,
	v vault.Vault,
) *runnerService {
	t.Helper()
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	return &runnerService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     v,
		os:        domain.OSLinuxAMD64,
	}
}

// makeTestManifest returns a minimal ArrowManifest with the given name.
// It includes a compiled target for OSLinuxAMD64 so resolveTarget succeeds.
func makeTestManifest(name string) *domain.ArrowManifest {
	return &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{
			Name:        name,
			Version:     "1.0.0",
			Description: "A test arrow",
			Tags:        []string{"test"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Execute:   domainStep.StepList{makeRunStep()},
					Install:   domainStep.StepList{makeRunStep()},
					Uninstall: domainStep.StepList{makeRunStep()},
				},
			},
		},
	}
}

func makeRunStep() domainStep.Step {
	return domainStep.NewRunStep("", "echo test", false, "", false)
}

// seedArrowVersionsCmd injects CompiledTargets into an existing Arrow aggregate.
type seedArrowVersionsCmd struct {
	ns      domain.Namespace
	version string
}

func (c seedArrowVersionsCmd) AggregateID() string            { return c.ns.String() }
func (c seedArrowVersionsCmd) EventName() string              { return "arrow.versions_seeded" }
func (c seedArrowVersionsCmd) ShouldSnapshot() bool           { return false }
func (c seedArrowVersionsCmd) Validate(_ *domain.Arrow) error { return nil }
func (c seedArrowVersionsCmd) EmitEvent(current *domain.Arrow) domain.Arrow {
	arrow := *current
	arrow.Versions = map[string]domain.ArrowManifest{
		c.version: {
			Targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64: {
					Lifecycle: domain.TargetLifecycle{
						Execute:   domainStep.StepList{makeRunStep()},
						Uninstall: domainStep.StepList{makeRunStep()},
					},
				},
			},
		},
	}
	return arrow
}

// addArrowForTest seeds an arrow aggregate into axArrow with compiled targets.
func addArrowForTest(
	t *testing.T,
	r *runnerService,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) {
	t.Helper()
	_, err := r.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	})
	require.NoError(t, err)
	r.axArrow.WaitPublish()

	version := manifest.Version
	if version == "" {
		version = "1.0.0"
	}
	_, err = r.axArrow.Send(context.Background(), seedArrowVersionsCmd{ns: ns, version: version})
	require.NoError(t, err)
	r.axArrow.WaitPublish()
}

// endExecutionWithVarsCmd injects a LastReturn with specific variables for testing.
type endExecutionWithVarsCmd struct {
	ns   domain.Namespace
	vars map[string]string
}

func (c endExecutionWithVarsCmd) AggregateID() string                          { return c.ns.String() }
func (c endExecutionWithVarsCmd) EventName() string                            { return "runtime.mock_end" }
func (c endExecutionWithVarsCmd) ShouldSnapshot() bool                         { return false }
func (c endExecutionWithVarsCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c endExecutionWithVarsCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.ns,
		State: domain.ArrowStateReady,
		LastReturn: &domainRuntime.Return{
			Method:    "_execute",
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: c.vars,
		},
	}
}

// wizardRunRequest builds a wizard.RunRequest from a runtime aggregate.
func wizardRunRequest(
	ns domain.Namespace,
	rt domainRuntime.ArrowRuntime,
) wizard.RunRequest {
	steps := make([]domainStep.Step, 0, len(rt.Execution.Steps))
	for _, sp := range rt.Execution.Steps {
		steps = append(steps, sp.Step)
	}
	return wizard.RunRequest{
		Namespace: ns,
		Variables: rt.Execution.Variables,
		Steps:     steps,
	}
}

// wireWizardExecutor subscribes a handler to "runtime.begun" that runs the wizard
// and emits EndExecution. This mirrors what projections.WizardExecutor does.
func wireWizardExecutor(
	t *testing.T,
	r *runnerService,
	wiz *mocks.Wizard,
) {
	t.Helper()
	_, err := r.axRuntime.Subscribe(
		"runtime.begun",
		func(_ context.Context, evt asynxModels.Event[domainRuntime.ArrowRuntime]) {
			rt := evt.Aggregate
			if rt.Execution == nil {
				return
			}
			ns := rt.Ref

			var execErr error
			if wiz != nil {
				execErr = wiz.Execute(context.Background(), wizardRunRequest(ns, rt), nil)
			}

			outcome := domainRuntime.ExecutionOutcomeSuccess
			if execErr != nil {
				if errors.Is(execErr, context.Canceled) {
					outcome = domainRuntime.ExecutionOutcomeCancelled
				} else {
					outcome = domainRuntime.ExecutionOutcomeFailed
				}
			}
			_, _ = r.axRuntime.Send(context.Background(), arrowcmds.EndExecution{
				Namespace: ns,
				Outcome:   outcome,
			})
		},
	)
	require.NoError(t, err)
}

// --- mapOutcomeToError ---

func TestMapOutcomeToError_Success_ReturnsNil(t *testing.T) {
	r := &runnerService{}
	assert.NoError(t, r.mapOutcomeToError(domainRuntime.ExecutionOutcomeSuccess))
}

func TestMapOutcomeToError_Cancelled_ReturnsContextCanceled(t *testing.T) {
	r := &runnerService{}
	assert.ErrorIs(t, r.mapOutcomeToError(domainRuntime.ExecutionOutcomeCancelled), context.Canceled)
}

func TestMapOutcomeToError_Failed_ReturnsError(t *testing.T) {
	r := &runnerService{}
	assert.Error(t, r.mapOutcomeToError(domainRuntime.ExecutionOutcomeFailed))
}

// --- resolveVariables ---

func TestResolveVariables_ReturnsBuiltins(t *testing.T) {
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: makeTestManifest("A")},
		GetArrowPath:  "/home/arrow",
	}
	r := testRunner(t, mv)
	r.os = domain.OSDarwinAMD64

	manifest := &domain.ArrowManifest{}
	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "github.com/org/repo", vars["ARROW_NAMESPACE"])
	assert.Equal(t, domain.OSDarwinAMD64.String(), vars["PLATFORM"])
	assert.Equal(t, "/home/arrow", vars["INSTALL_PATH"])
	assert.Equal(t, filepath.Dir("/home/arrow"), vars["WORKDIR"])
}

func TestResolveVariables_UserVarsOverrideManifestDefaults(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: "MY_VAR", Default: "default_val"},
		},
	}
	userVars := map[string]string{"MY_VAR": "user_val"}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", userVars)
	require.NoError(t, err)
	assert.Equal(t, "user_val", vars["MY_VAR"])
}

func TestResolveVariables_ManifestDefaultsApplied(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: "TIMEOUT", Default: "30"},
		},
	}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "30", vars["TIMEOUT"])
}

func TestResolveVariables_NetbridgePortsAdded(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	nb := &mocks.Netbridge{AllocatePort: 5000}
	r := testRunner(t, mv)
	r.netbridge = nb
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: false},
		},
	}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "5000", vars["HTTP_PORT"])
}

func TestResolveVariables_NetbridgeRequired_ErrorReturns(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	nb := &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}
	r := testRunner(t, mv)
	r.netbridge = nb
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: true},
		},
	}

	_, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.Error(t, err)
}

func TestResolveVariables_NetbridgeOptional_ErrorSkipped(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	nb := &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}
	r := testRunner(t, mv)
	r.netbridge = nb
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: false},
		},
	}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	_, exists := vars["HTTP_PORT"]
	assert.False(t, exists)
}

func TestResolveVariables_StoredVarsFromLastReturn(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.os = domain.OSLinuxAMD64

	ns := domain.Namespace("github.com/org/repo")

	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)

	storedVars := map[string]string{"STORED_KEY": "stored_val"}
	_, err = r.axRuntime.Send(context.Background(), endExecutionWithVarsCmd{
		ns:   ns,
		vars: storedVars,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	manifest := &domain.ArrowManifest{}
	vars, err := r.resolveVariables(context.Background(), ns, manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "stored_val", vars["STORED_KEY"])
}

func TestResolveVariables_NilNetbridge_Skipped(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.netbridge = nil
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: true},
		},
	}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/repo", manifest, domain.Target{}, "_execute", nil)
	require.NoError(t, err)
	_, exists := vars["HTTP_PORT"]
	assert.False(t, exists)
}

// --- Stop ---

func TestStop_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	err := r.Stop(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestStop_StateNotRunning_ReturnsErrStateViolation(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	ns := domain.Namespace("github.com/org/repo")
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.Stop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestStop_StateRunning_SendsMarkStopping(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	ns := domain.Namespace("github.com/org/repo")
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.Stop(context.Background(), ns)
	require.NoError(t, err)

	r.axRuntime.WaitPublish()

	runtime, err := r.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, runtime.State)
}

// --- ExecuteSync ---

func TestExecuteSync_ArrowNotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	err := r.ExecuteSync(context.Background(), "github.com/org/repo", "_execute", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestExecuteSync_ResolveVariablesError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"},
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: true},
		},
	}
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.netbridge = &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}

	addArrowForTest(t, r, ns, manifest)

	err := r.ExecuteSync(context.Background(), ns, "_execute", nil)
	require.Error(t, err)
}

func TestExecuteSync_SendWaitValidationError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	addArrowForTest(t, r, ns, manifest)

	// _execute requires AvailableIn=[Ready], but no runtime seeded → SendWait returns validation error
	err := r.ExecuteSync(context.Background(), ns, "_execute", nil)
	require.Error(t, err)
}

func TestExecuteSync_HappyPath_WizardSucceeds(t *testing.T) {
	manifest := makeTestManifest("Arrow1")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/arrow1",
	}
	r := testRunner(t, mv)
	wiz := &mocks.Wizard{}
	wireWizardExecutor(t, r, wiz)

	ns := domain.Namespace("github.com/test/arrow1")
	addArrowForTest(t, r, ns, manifest)

	// Seed runtime to Ready so _execute is allowed
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.ExecuteSync(context.Background(), ns, "_execute", nil)
	require.NoError(t, err)

	rt, err := r.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, rt.LastReturn)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
}

func TestExecuteSync_WizardFails_ReturnsMappedError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	wiz := &mocks.Wizard{ExecuteErr: errors.New("step failed")}
	wireWizardExecutor(t, r, wiz)

	ns := domain.Namespace("github.com/test/arrow2")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow2", Version: "1.0.0"}}
	addArrowForTest(t, r, ns, manifest)

	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.ExecuteSync(context.Background(), ns, "_execute", nil)
	require.Error(t, err)
}

// --- BeginExecution ---

func TestBeginExecution_ArrowNotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	err := r.BeginExecution(context.Background(), "github.com/org/repo", "_execute", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestBeginExecution_Success_EmitsRunningState(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := makeTestManifest("A")

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	r := testRunner(t, mv)
	addArrowForTest(t, r, ns, manifest)

	err := r.BeginExecution(context.Background(), ns, "_install", nil)
	require.NoError(t, err)

	r.axRuntime.WaitPublish()

	rt, err := r.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, rt.State)
}

func TestBeginExecution_StateViolation_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"}}

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	addArrowForTest(t, r, ns, manifest)

	// Runtime exists but state is Running — _execute validation rejects it
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.BeginExecution(context.Background(), ns, "_execute", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestBeginExecution_AsynxValidationError_ReturnsErrStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := makeTestManifest("A")

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	r := testRunner(t, mv)
	addArrowForTest(t, r, ns, manifest)

	// Arrow already has an active run — BeginExecution.Validate rejects with ErrValidation
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmdWithExecution{
		NS:        ns,
		State:     domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{Method: "_execute"},
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	err = r.BeginExecution(context.Background(), ns, "_execute", nil)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- SetPostExecutionHook ---

func TestSetPostExecutionHook_SetsHook(t *testing.T) {
	r := &runnerService{}
	assert.Nil(t, r.hook)

	called := false
	r.SetPostExecutionHook(func(_ context.Context, _ domain.Namespace, _ string, _ error, _ domainRuntime.ExecutionOutcome) {
		called = true
	})
	assert.NotNil(t, r.hook)

	r.hook(context.Background(), "github.com/org/repo", "_execute", nil, domainRuntime.ExecutionOutcomeSuccess)
	assert.True(t, called)
}

// --- New ---

func TestNew_ReturnsHookableRunner(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r, err := New(axArrow, axRuntime, mv, nil, nil, domain.OSLinuxAMD64)
	require.NoError(t, err)
	assert.NotNil(t, r)

	// Verify it implements HookableRunner
	var _ HookableRunner = r
}

// --- stepsForMethod ---

func TestStepsForMethod_Install_PrependsDependenciesStep(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Install: domainStep.StepList{},
		},
	}

	steps, availableIn, err := r.stepsForMethod(target, "_install")
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, domainStep.StepTypeDependencies, steps[0].Type())
	assert.Nil(t, availableIn)
}

func TestStepsForMethod_Uninstall_ReturnsReadyAvailableIn(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{}

	_, availableIn, err := r.stepsForMethod(target, "_uninstall")
	require.NoError(t, err)
	require.Len(t, availableIn, 1)
	assert.Equal(t, domain.ArrowStateReady, availableIn[0])
}

func TestStepsForMethod_Execute_WithSteps_ReturnsReadyAvailableIn(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Execute: domainStep.StepList{makeRunStep()},
		},
	}

	_, availableIn, err := r.stepsForMethod(target, "_execute")
	require.NoError(t, err)
	require.Len(t, availableIn, 1)
	assert.Equal(t, domain.ArrowStateReady, availableIn[0])
}

func TestStepsForMethod_Execute_NoSteps_ReturnsErrMethodNotFound(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{}

	_, _, err := r.stepsForMethod(target, "_execute")
	require.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_Stop_WithSteps_ReturnsReadyAvailableIn(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{
		Lifecycle: domain.TargetLifecycle{
			Stop: domainStep.StepList{makeRunStep()},
		},
	}

	_, availableIn, err := r.stepsForMethod(target, "_stop")
	require.NoError(t, err)
	require.Len(t, availableIn, 1)
	assert.Equal(t, domain.ArrowStateReady, availableIn[0])
}

func TestStepsForMethod_Stop_NoSteps_ReturnsErrMethodNotFound(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{}

	_, _, err := r.stepsForMethod(target, "_stop")
	require.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

func TestStepsForMethod_CustomMethod_ReturnsMethodSteps(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{
		Methods: map[string]domain.Method{
			"my_method": {
				Steps:       domainStep.StepList{makeRunStep()},
				AvailableIn: []domain.ArrowState{domain.ArrowStateReady},
			},
		},
	}

	steps, availableIn, err := r.stepsForMethod(target, "my_method")
	require.NoError(t, err)
	assert.NotNil(t, steps)
	require.Len(t, availableIn, 1)
	assert.Equal(t, domain.ArrowStateReady, availableIn[0])
}

func TestStepsForMethod_CustomMethod_Unknown_ReturnsErrMethodNotFound(t *testing.T) {
	r := &runnerService{}
	target := domain.Target{}

	_, _, err := r.stepsForMethod(target, "nonexistent")
	require.ErrorIs(t, err, apperrors.ErrMethodNotFound)
}

// --- dep built-ins in resolveVariables ---

func TestResolveVariables_DepBuiltins_AddedWhenVaultHit(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	mv := &vaultByNS{
		entries: map[domain.Namespace]*vault.VaultEntry{
			depNs: {Manifest: makeTestManifest("Dep")},
		},
		paths: map[domain.Namespace]string{
			depNs: "/home/dep",
		},
	}
	r := testRunner(t, mv)
	r.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{}
	target := domain.Target{
		Tools: []domain.Namespace{depNs},
	}

	vars, err := r.resolveVariables(context.Background(), "github.com/org/root", manifest, target, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "/home/dep", vars[depNs.String()+".INSTALL_PATH"])
}

// vaultByNS is a test vault that returns different entries per namespace.
type vaultByNS struct {
	entries map[domain.Namespace]*vault.VaultEntry
	paths   map[domain.Namespace]string
}

func (v *vaultByNS) GetArrow(_ context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	entry, ok := v.entries[ns]
	if !ok {
		return nil, "", vault.ErrNotCached
	}
	return entry, v.paths[ns], nil
}

func (v *vaultByNS) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.ArrowManifest, _ []domain.Namespace) (string, error) {
	return "/home/test", nil
}

func (v *vaultByNS) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *vaultByNS) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *vaultByNS) DeleteArrow(_ context.Context, _ domain.Namespace) error  { return nil }
func (v *vaultByNS) DeleteQuiver(_ context.Context, _ domain.Namespace) error { return nil }
func (v *vaultByNS) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// --- error injection stubs for asynx ---

// errArrow is a minimal asynx.Asynx[domain.Arrow] that returns a fixed error from Get.
type errArrow struct {
	getErr error
	inner  asynx.Asynx[domain.Arrow]
}

func (e *errArrow) Send(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	if e.inner != nil {
		return e.inner.Send(ctx, cmd)
	}
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (e *errArrow) SendWait(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return e.Send(ctx, cmd)
}
func (e *errArrow) Shutdown(_ context.Context) error { return nil }
func (e *errArrow) Get(ctx context.Context, id string) (domain.Arrow, error) {
	if e.getErr != nil {
		return domain.Arrow{}, e.getErr
	}
	if e.inner != nil {
		return e.inner.Get(ctx, id)
	}
	return domain.Arrow{}, asynxModels.ErrNotFound
}
func (e *errArrow) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (e *errArrow) Preload(_ context.Context, _ string) error        { return nil }
func (e *errArrow) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", nil
}
func (e *errArrow) Unsubscribe(_ string) error { return nil }
func (e *errArrow) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (e *errArrow) Forget(_ context.Context, _ string) error { return nil }
func (e *errArrow) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "forget-sub-id", nil
}
func (e *errArrow) WaitPublish() {}

// errRuntime is a minimal asynx.Asynx[domainRuntime.ArrowRuntime] with injectable errors.
type errRuntime struct {
	getErr  error
	sendErr error
	inner   asynx.Asynx[domainRuntime.ArrowRuntime]
}

func (e *errRuntime) Send(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	if e.sendErr != nil {
		return asynxModels.Event[domainRuntime.ArrowRuntime]{}, e.sendErr
	}
	if e.inner != nil {
		return e.inner.Send(ctx, cmd)
	}
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (e *errRuntime) SendWait(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return e.Send(ctx, cmd)
}
func (e *errRuntime) Shutdown(_ context.Context) error { return nil }
func (e *errRuntime) Get(ctx context.Context, id string) (domainRuntime.ArrowRuntime, error) {
	if e.getErr != nil {
		return domainRuntime.ArrowRuntime{}, e.getErr
	}
	if e.inner != nil {
		return e.inner.Get(ctx, id)
	}
	return domainRuntime.ArrowRuntime{}, asynxModels.ErrNotFound
}
func (e *errRuntime) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (e *errRuntime) Preload(_ context.Context, _ string) error        { return nil }
func (e *errRuntime) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	return "", nil
}
func (e *errRuntime) Unsubscribe(_ string) error { return nil }
func (e *errRuntime) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}
func (e *errRuntime) Forget(_ context.Context, _ string) error { return nil }
func (e *errRuntime) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
}
func (e *errRuntime) WaitPublish() {}

// --- BeginExecution non-ErrNotFound error ---

func TestBeginExecution_ResolveVariablesError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "A", Version: "1.0.0"},
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: true},
		},
	}
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	r.netbridge = &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}
	addArrowForTest(t, r, ns, manifest)

	err := r.BeginExecution(context.Background(), ns, "_execute", nil)
	require.Error(t, err)
}

func TestBeginExecution_ArrowGetGenericError_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	genericErr := errors.New("arrow store failure")
	r.axArrow = &errArrow{getErr: genericErr}

	err := r.BeginExecution(context.Background(), "github.com/org/repo", "_execute", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, genericErr)
}

// --- ExecuteSync non-ErrNotFound error and runtime.Get error ---

func TestExecuteSync_ArrowGetGenericError_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	genericErr := errors.New("arrow store failure")
	r.axArrow = &errArrow{getErr: genericErr}

	err := r.ExecuteSync(context.Background(), "github.com/org/repo", "_execute", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, genericErr)
}

func TestExecuteSync_RuntimeGetErrorAfterSendWait_ReturnsError(t *testing.T) {
	// SendWait returns success (validation error triggers immediately, so use _install
	// which has nil AvailableIn — always valid). Then axRuntime.Get returns an error.
	// We need SendWait to succeed and then Get to fail.
	// Use an errRuntime that wraps the real runtime but returns getErr on Get after SendWait.
	manifest := makeTestManifest("A")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	r := testRunner(t, mv)

	ns := domain.Namespace("github.com/org/repo")
	addArrowForTest(t, r, ns, manifest)

	realRuntime := r.axRuntime
	genericErr := errors.New("storage failure")
	r.axRuntime = &errRuntime{
		getErr: genericErr,
		inner:  realRuntime, // SendWait delegates to inner
	}

	err := r.ExecuteSync(context.Background(), ns, "_install", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, genericErr)
}

// --- Stop non-ErrNotFound error ---

func TestStop_RuntimeGetGenericError_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)
	genericErr := errors.New("storage failure")
	r.axRuntime = &errRuntime{getErr: genericErr}

	err := r.Stop(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, genericErr)
}

func TestStop_MarkStoppingSendError_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	r := testRunner(t, mv)

	ns := domain.Namespace("github.com/org/repo")
	_, err := r.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()

	genericErr := errors.New("send failure")
	realRuntime := r.axRuntime
	r.axRuntime = &errRuntime{
		sendErr: genericErr,
		inner:   realRuntime,
	}

	err = r.Stop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, genericErr)
}
