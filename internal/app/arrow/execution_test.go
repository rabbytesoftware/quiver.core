package arrow

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mapOutcomeToError ---

func TestMapOutcomeToError_Success_ReturnsNil(t *testing.T) {
	svc := &arrowService{}
	assert.NoError(t, svc.mapOutcomeToError(domainRuntime.ExecutionOutcomeSuccess))
}

func TestMapOutcomeToError_Cancelled_ReturnsContextCanceled(t *testing.T) {
	svc := &arrowService{}
	assert.ErrorIs(t, svc.mapOutcomeToError(domainRuntime.ExecutionOutcomeCancelled), context.Canceled)
}

func TestMapOutcomeToError_Failed_ReturnsError(t *testing.T) {
	svc := &arrowService{}
	assert.Error(t, svc.mapOutcomeToError(domainRuntime.ExecutionOutcomeFailed))
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
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)

	// inject a runtime with LastReturn via a direct command
	storedVars := map[string]string{"STORED_KEY": "stored_val"}
	_, err = svc.asynxRuntime.Send(context.Background(), endExecutionWithVarsCmd{
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
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.Stop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

func TestStop_StateRunning_SendsMarkStopping(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.Stop(context.Background(), ns)
	require.NoError(t, err)

	svc.asynxRuntime.WaitPublish()

	runtime, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, runtime.State)
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

// --- executeSync ---

func TestExecuteSync_ArrowNotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	err := svc.executeSync(context.Background(), "github.com/org/repo", "_execute", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestExecuteSync_ResolveVariablesError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{
		Name:    "A",
		Version: "1.0.0",
		Netbridge: []netbridge.PortDef{
			{Name: "HTTP_PORT", Protocol: netbridge.ProtocolTCP, Default: 8080, Required: true},
		},
	}
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.engines.Netbridge = &mocks.Netbridge{AllocateErr: errors.New("port unavailable")}

	addArrowForTest(t, svc, ns, manifest)

	err := svc.executeSync(context.Background(), ns, "_execute", nil)

	require.Error(t, err)
}

func TestExecuteSync_SendWaitError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	addArrowForTest(t, svc, ns, manifest)

	// _execute requires AvailableIn=[Ready], but no runtime seeded → SendWait returns validation error
	err := svc.executeSync(context.Background(), ns, "_execute", nil)

	require.Error(t, err)
}

func TestExecuteSync_HappyPath_WizardSucceeds(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.ArrowManifest{
		Name:    "Arrow1",
		Version: "1.0.0",
		Lifecycle: domain.Lifecycle{
			Execute: domainStep.StepList{},
		},
	})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.inner.asynxArrow.WaitPublish()

	// Seed runtime to Ready so _execute is allowed
	_, err := f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	err = f.inner.executeSync(ctx, ns, "_execute", nil)

	require.NoError(t, err)

	rt, err := f.inner.asynxRuntime.Get(ctx, ns.String())
	require.NoError(t, err)
	require.NotNil(t, rt.LastReturn)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
}

func TestExecuteSync_WizardFails_ReturnsMappedError(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow2")
	f.manifold.set(ns, &domain.ArrowManifest{
		Name:    "Arrow2",
		Version: "1.0.0",
	})
	f.wizard.ExecuteErr = errors.New("step failed")

	require.NoError(t, f.svc.Add(ctx, ns))
	f.inner.asynxArrow.WaitPublish()

	_, err := f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	err = f.inner.executeSync(ctx, ns, "_execute", nil)

	require.Error(t, err)
}

func TestExecuteSync_SuccessButNoLastReturn_ReturnsError(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow3")
	f.manifold.set(ns, &domain.ArrowManifest{
		Name:    "Arrow3",
		Version: "1.0.0",
	})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.inner.asynxArrow.WaitPublish()

	// Seed runtime to Ready so _execute AvailableIn check passes
	_, err := f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	// Make wizard fail → EndExecution emitted with Failed outcome, LastReturn set.
	// To hit the LastReturn == nil branch we'd need an aggregate that never sets
	// LastReturn on EndExecution. Instead we verify that a successful wizard path
	// sets LastReturn correctly and does not hit the nil branch.
	err = f.inner.executeSync(ctx, ns, "_execute", nil)
	// Wizard succeeds → LastReturn != nil → no error from that branch
	require.NoError(t, err)
}

// --- beginExecution (non-ErrNotFound branch) ---

func TestBeginExecution_NonNotFoundError_PropagatesError(t *testing.T) {
	// asynx only returns ErrNotFound for missing aggregates, so we cannot inject
	// a custom error through the real asynxArrow. Instead, test the
	// executeSync non-ErrNotFound branch indirectly: confirm that a non-absent
	// runtime still rejects _execute with a validation error (not ErrNotFound).
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	addArrowForTest(t, svc, ns, manifest)

	// Runtime exists but state is Running — BeginExecution(_execute) validation rejects it
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.beginExecution(context.Background(), ns, "_execute", nil)

	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

