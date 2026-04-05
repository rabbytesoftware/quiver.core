package arrow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mapOutcome ---

func TestMapOutcome_Nil_ReturnsSuccess(t *testing.T) {
	svc := &arrowService{}
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, svc.mapOutcome(nil))
}

func TestMapOutcome_ContextCanceled_ReturnsCancelled(t *testing.T) {
	svc := &arrowService{}
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, svc.mapOutcome(context.Canceled))
}

func TestMapOutcome_OtherError_ReturnsFailed(t *testing.T) {
	svc := &arrowService{}
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, svc.mapOutcome(errors.New("boom")))
}

// --- resolveVariables ---

func TestResolveVariables_ReturnsBuiltins(t *testing.T) {
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: makeTestManifest("A")},
		GetArrowPath:  "/home/arrow",
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.os = domain.OSDarwinAMD64

	manifest := &domain.ArrowManifest{}
	vars, err := svc.resolveVariables(context.Background(), "github.com/org/repo", manifest, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "github.com/org/repo", vars["ARROW_NAMESPACE"])
	assert.Equal(t, domain.OSDarwinAMD64.String(), vars["PLATFORM"])
	assert.Equal(t, "/home/arrow", vars["INSTALL_PATH"])
}

func TestResolveVariables_UserVarsOverrideManifestDefaults(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: "MY_VAR", Default: "default_val"},
		},
	}
	userVars := map[string]string{"MY_VAR": "user_val"}

	vars, err := svc.resolveVariables(context.Background(), "github.com/org/repo", manifest, "_execute", userVars)
	require.NoError(t, err)
	assert.Equal(t, "user_val", vars["MY_VAR"])
}

func TestResolveVariables_ManifestDefaultsApplied(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Variables: []domain.Variable{
			{Name: "TIMEOUT", Default: "30"},
		},
	}

	vars, err := svc.resolveVariables(context.Background(), "github.com/org/repo", manifest, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "30", vars["TIMEOUT"])
}

func TestResolveVariables_NetbridgePortsAdded(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	nb := &mocks.Netbridge{AllocatePort: 5000}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.engines.Netbridge = nb
	svc.os = domain.OSLinuxAMD64

	manifest := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: false},
		},
	}

	vars, err := svc.resolveVariables(context.Background(), "github.com/org/repo", manifest, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "5000", vars["HTTP_PORT"])
}

func TestResolveVariables_StoredVarsFromLastReturn(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)
	svc.os = domain.OSLinuxAMD64

	ns := domain.Namespace("github.com/org/repo")

	// seed a LastReturn in the runtime
	err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)

	// inject a runtime with LastReturn via a direct command
	storedVars := map[string]string{"STORED_KEY": "stored_val"}
	err = svc.asynxRuntime.Send(context.Background(), endExecutionWithVarsCmd{
		ns:   ns,
		vars: storedVars,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	manifest := &domain.ArrowManifest{}
	vars, err := svc.resolveVariables(context.Background(), ns, manifest, "_execute", nil)
	require.NoError(t, err)
	assert.Equal(t, "stored_val", vars["STORED_KEY"])
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
		Namespace: c.ns,
		State:     domain.ArrowStateReady,
		LastReturn: &domainRuntime.Return{
			Method:    "_execute",
			Outcome:   domainRuntime.ExecutionOutcomeSuccess,
			Variables: c.vars,
		},
	}
}

// --- Stop ---

func TestStop_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	err := svc.Stop(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStop_StateNotRunning_ReturnsErrStateViolation(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Stop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

func TestStop_StateRunning_SendsMarkStopping(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Stop(context.Background(), ns)
	require.NoError(t, err)

	svc.asynxRuntime.WaitPublish()

	runtime, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, runtime.State)
}

// --- HandleExecutionError ---

func TestHandleExecutionError_NonExecuteMethod_NoStop(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	mw := &mocks.Wizard{}
	svc.engines.Wizard = mw

	svc.handleExecutionError(context.Background(), "github.com/org/repo", "_install", context.Canceled)

	// No stop should be dispatched for _install
	assert.False(t, mw.WasExecuteCalled())
}

func TestHandleExecutionError_ExecuteMethodNotCanceled_NoStop(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	mw := &mocks.Wizard{}
	svc.engines.Wizard = mw

	svc.handleExecutionError(context.Background(), "github.com/org/repo", "_execute", errors.New("other error"))

	assert.False(t, mw.WasExecuteCalled())
}

func TestHandleExecutionError_ExecuteCanceled_WithStopLifecycle_DispatchesStop(t *testing.T) {
	stopStep := domainStep.NewDependenciesStep("stop")
	manifest := &domain.ArrowManifest{
		Name:    "TestArrow",
		Version: "1.0.0",
		Lifecycle: domain.Lifecycle{
			Stop: domainStep.StepList{stopStep},
		},
	}

	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)
	mw := &mocks.Wizard{}
	svc.engines.Wizard = mw

	ns := domain.Namespace("github.com/org/repo")
	addArrowForTest(t, svc, ns, manifest)

	// Set runtime to ready — handleExecutionError is called after EndExecution sets state to ready
	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	svc.handleExecutionError(context.Background(), ns, "_execute", context.Canceled)

	// Wait for goroutine
	time.Sleep(100 * time.Millisecond)

	assert.True(t, mw.WasExecuteCalled())
}

func TestHandleExecutionError_ExecuteCanceled_NoStopLifecycle_NoStop(t *testing.T) {
	manifest := &domain.ArrowManifest{
		Name:    "TestArrow",
		Version: "1.0.0",
	}

	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)
	mw := &mocks.Wizard{}
	svc.engines.Wizard = mw

	ns := domain.Namespace("github.com/org/repo")
	addArrowForTest(t, svc, ns, manifest)

	svc.handleExecutionError(context.Background(), ns, "_execute", context.Canceled)

	time.Sleep(50 * time.Millisecond)
	assert.False(t, mw.WasExecuteCalled())
}

// --- resolveManifest ---

func TestResolveManifest_FreshVaultHit_ReturnsManifestDirectly(t *testing.T) {
	manifest := makeTestManifest("FreshArrow")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/fresh",
		GetArrowErr:   nil,
	}
	mm := &mocks.Manifold{}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, manifest, got)
	assert.Equal(t, "/home/fresh", path)
	assert.Equal(t, 0, mv.PutArrowCalls)
}

func TestResolveManifest_StaleVaultManifoldSucceeds_ReturnsFreshManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleArrow")
	freshManifest := makeTestManifest("FreshArrow")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: staleManifest},
		GetArrowPath:  "/home/stale",
		GetArrowErr:   vault.ErrStale,
		PutArrowPath:  "/home/fresh",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: freshManifest}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/fresh", path)
	assert.Equal(t, 1, mv.PutArrowCalls)
}

func TestResolveManifest_StaleVaultManifoldFails_ReturnsStaleManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleArrow")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: staleManifest},
		GetArrowPath:  "/home/stale",
		GetArrowErr:   vault.ErrStale,
	}
	mm := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, staleManifest, got)
	assert.Equal(t, "/home/stale", path)
	assert.Equal(t, 0, mv.PutArrowCalls)
}

func TestResolveManifest_NotCachedManifoldSucceeds_ReturnsManifestAndStores(t *testing.T) {
	freshManifest := makeTestManifest("NewArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/home/new",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: freshManifest}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/new", path)
	assert.Equal(t, 1, mv.PutArrowCalls)
}

func TestResolveManifest_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	mm := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_StaleVaultPutFails_ReturnsError(t *testing.T) {
	staleManifest := makeTestManifest("StaleArrow")
	freshManifest := makeTestManifest("FreshArrow")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: staleManifest},
		GetArrowPath:  "/home/stale",
		GetArrowErr:   vault.ErrStale,
		PutArrowErr:   errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveArrowManifest: freshManifest}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_NotCachedPutFails_ReturnsError(t *testing.T) {
	freshManifest := makeTestManifest("NewArrow")
	mv := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
		PutArrowErr: errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveArrowManifest: freshManifest}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{
		GetArrowErr: unexpectedErr,
	}
	mm := &mocks.Manifold{}
	svc := makeArrowServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

// --- buildDepResolver ---

func TestBuildDepResolver_ReturnsResolverThatReturnsDeps(t *testing.T) {
	manifest := makeTestManifest("DepArrow")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/dep",
		GetArrowErr:   nil,
	}
	mm := &mocks.Manifold{}
	svc := makeArrowServiceWithMocks(mv, mm)

	resolver := svc.buildDepResolver()
	deps, err := resolver(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, manifest.Dependencies, deps)
}

func TestBuildDepResolver_ManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	mm := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}
	svc := makeArrowServiceWithMocks(mv, mm)

	resolver := svc.buildDepResolver()
	deps, err := resolver(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, deps)
}
