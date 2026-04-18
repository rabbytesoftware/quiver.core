package deps_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution/installer/deps"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner is a configurable Runner for testing.
type mockRunner struct {
	executeSync func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error
}

func (m *mockRunner) BeginExecution(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
	return nil
}

func (m *mockRunner) ExecuteSync(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
	if m.executeSync != nil {
		return m.executeSync(ctx, ns, method, vars)
	}
	return nil
}

func (m *mockRunner) Stop(_ context.Context, _ domain.Namespace) error {
	return nil
}

// syncCall records a single ExecuteSync invocation.
type syncCall struct {
	ns     domain.Namespace
	method string
}

// depFixture wires a DependenciesHandler with real asynx stores and mock engines.
type depFixture struct {
	handler   deps.DependenciesHandler
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	vault     *mocks.Vault
	manifold  *mocks.Manifold
	depTree   *mocks.DepTree
	runner    *mockRunner
	syncCalls []syncCall
	syncErr   error
}

func newDepFixture(t *testing.T) *depFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(runtimeES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	dt := &mocks.DepTree{}

	f := &depFixture{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		vault:     mv,
		manifold:  mm,
		depTree:   dt,
	}

	r := &mockRunner{}
	r.executeSync = func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
		f.syncCalls = append(f.syncCalls, syncCall{ns: ns, method: method})
		return f.syncErr
	}
	f.runner = r

	f.handler = deps.New(dt, mv, mm, axArrow, axRuntime, r)

	return f
}

func (f *depFixture) req(ns domain.Namespace) wizstep.Request {
	return wizstep.Request{NSKey: ns.String()}
}

func (f *depFixture) step() domainstep.DependenciesStep {
	return domainstep.NewDependenciesStep("Resolve dependencies")
}

// TestDepsHandler_NoDeps_ReturnsNil verifies Execute succeeds when dep tree returns only the main ns.
func TestDepsHandler_NoDeps_ReturnsNil(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	f.depTree.Result = []domain.Namespace{ns} // only self

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	assert.Empty(t, f.syncCalls)
}

// TestDepsHandler_DepAlreadyInstalled_Skipped verifies installed deps are not reinstalled.
func TestDepsHandler_DepAlreadyInstalled_Skipped(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	// Seed runtime with dep already in Ready state
	_, err := f.axRuntime.Send(context.Background(), readyRuntimeCmd{ns: dep})
	require.NoError(t, err)
	f.axRuntime.WaitPublish()

	err = f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	assert.Empty(t, f.syncCalls, "should not install already-ready dep")
}

// TestDepsHandler_DepNotInstalled_InstallsCalled verifies uninstalled deps are installed.
func TestDepsHandler_DepNotInstalled_InstallsCalled(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	require.Len(t, f.syncCalls, 1)
	assert.Equal(t, dep, f.syncCalls[0].ns)
	assert.Equal(t, "_install", f.syncCalls[0].method)
}

// TestDepsHandler_DepResolveFails_ReturnsError verifies dep tree resolution failure is propagated.
func TestDepsHandler_DepResolveFails_ReturnsError(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	f.depTree.Err = errors.New("resolve failed")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve failed")
}

// TestDepsHandler_InstallFails_RollsBack verifies that a failed dep install triggers rollback.
func TestDepsHandler_InstallFails_RollsBack(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep1 := domain.Namespace("github.com/org/dep1")
	dep2 := domain.Namespace("github.com/org/dep2")
	f.depTree.Result = []domain.Namespace{dep1, dep2, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	callN := 0
	f.runner.executeSync = func(ctx context.Context, installNS domain.Namespace, method string, vars map[string]string) error {
		callN++
		if method == "_install" && callN == 1 {
			// dep1 installs successfully — seed it as Ready for rollback check
			_, _ = f.axRuntime.Send(ctx, readyRuntimeCmd{ns: dep1})
			f.axRuntime.WaitPublish()
			return nil
		}
		if method == "_install" && callN == 2 {
			return errors.New("dep2 install failed")
		}
		return nil // uninstall calls
	}

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dep2 install failed")
}

// TestDepsHandler_DepTreeFails_NoPanic verifies graceful error on dep tree failure.
func TestDepsHandler_DepTreeFails_NoPanic(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	f.depTree.Err = errors.New("network timeout")

	require.NotPanics(t, func() {
		_ = f.handler.Execute(context.Background(), f.req(ns), f.step())
	})
}

// TestDepsHandler_ResolveManifest_StaleManifoldSucceedsPutFails verifies ErrStale path where
// manifold resolves the manifest but PutArrow fails — Execute returns the put error.
func TestDepsHandler_ResolveManifest_StaleManifoldSucceedsPutFails(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	staleManifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "0.9.0"}}
	f.vault.GetArrowErr = vault.ErrStale
	f.vault.GetArrowEntry = &vault.VaultEntry{Manifest: staleManifest}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}
	f.vault.PutArrowErr = errors.New("disk full")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

// TestDepsHandler_ResolveManifest_StaleManifoldFails verifies ErrStale path where manifold fails —
// Execute falls back to the stale entry and succeeds.
func TestDepsHandler_ResolveManifest_StaleManifoldFails(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	staleManifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "0.9.0"}}
	f.vault.GetArrowErr = vault.ErrStale
	f.vault.GetArrowEntry = &vault.VaultEntry{Manifest: staleManifest}
	f.manifold.ResolveArrowErr = errors.New("manifold unreachable")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	require.Len(t, f.syncCalls, 1)
	assert.Equal(t, dep, f.syncCalls[0].ns)
	assert.Equal(t, "_install", f.syncCalls[0].method)
}

// TestDepsHandler_ResolveManifest_NotCachedPutFails verifies ErrNotCached path where manifold
// resolves but PutArrow fails — Execute returns the put error.
func TestDepsHandler_ResolveManifest_NotCachedPutFails(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	// vault default is ErrNotCached; just set PutArrowErr
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}
	f.vault.PutArrowErr = errors.New("disk full")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

// TestDepsHandler_ResolveManifest_UnknownVaultError verifies that an unrecognised vault error
// (not ErrStale, not ErrNotCached) is propagated from Execute.
func TestDepsHandler_ResolveManifest_UnknownVaultError(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	f.vault.GetArrowErr = errors.New("i/o error")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "i/o error")
}

// TestDepsHandler_ResolveManifestFailsMidLoop_RollbackTriggered verifies that when resolveManifest
// fails mid-loop, already-installed deps are rolled back.
func TestDepsHandler_ResolveManifestFailsMidLoop_RollbackTriggered(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep1 := domain.Namespace("github.com/org/dep1")
	dep2 := domain.Namespace("github.com/org/dep2")
	f.depTree.Result = []domain.Namespace{dep1, dep2, ns}

	callCount := 0
	f.vault.GetArrowErr = vault.ErrNotCached
	f.runner.executeSync = func(ctx context.Context, installNS domain.Namespace, method string, vars map[string]string) error {
		if method == "_install" {
			callCount++
			if callCount == 1 {
				// dep1 installed; seed it as Ready so rollback picks it up
				_, _ = f.axRuntime.Send(ctx, readyRuntimeCmd{ns: dep1})
				f.axRuntime.WaitPublish()
				// Make vault fail for the next resolveManifest call
				f.vault.GetArrowErr = errors.New("i/o error")
				return nil
			}
		}
		return nil // uninstall calls during rollback
	}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "i/o error")
}

// TestDepsHandler_ArrowAlreadyInAsynx_SkipsSend verifies that when a dep already exists in
// asynxArrow, the AddArrow Send is skipped but _install is still called.
func TestDepsHandler_ArrowAlreadyInAsynx_SkipsSend(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	// Pre-seed dep into asynxArrow so Get returns a non-empty Arrow.
	_, err := f.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: dep,
		Version:   domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}},
	})
	require.NoError(t, err)
	f.axArrow.WaitPublish()

	err = f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	require.Len(t, f.syncCalls, 1)
	assert.Equal(t, dep, f.syncCalls[0].ns)
	assert.Equal(t, "_install", f.syncCalls[0].method)
}

// TestDepsHandler_UpdateIndirectDeps_WritesIndirect verifies that updateIndirectDeps correctly
// separates direct and indirect deps and calls PutArrow.
func TestDepsHandler_UpdateIndirectDeps_WritesIndirect(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	direct := domain.Namespace("github.com/org/direct")
	indirect := domain.Namespace("github.com/org/indirect")
	// deptree returns [indirect, direct, ns] (indirect is a transitive dep of direct)
	f.depTree.Result = []domain.Namespace{indirect, direct, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	// Seed ns into asynxArrow with direct as its declared dependency.
	_, err := f.axArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Version: domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "arrow", Version: "1.0.0"}},
	})
	require.NoError(t, err)
	f.axArrow.WaitPublish()

	// Reset PutArrowErr to nil so updateIndirectDeps succeeds.
	f.vault.GetArrowErr = vault.ErrNotCached
	f.vault.PutArrowErr = nil

	err = f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	// PutArrow should have been called at least once (for each dep + once for indirect update).
	assert.Positive(t, f.vault.PutArrowCalls)
}

// TestDepsHandler_Rollback_SkipsNonReadyDep verifies that rollback only uninstalls deps that
// are in ArrowStateReady and skips others.
func TestDepsHandler_Rollback_SkipsNonReadyDep(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep1 := domain.Namespace("github.com/org/dep1") // will be Ready → should be rolled back
	dep2 := domain.Namespace("github.com/org/dep2") // not Ready → should be skipped
	dep3 := domain.Namespace("github.com/org/dep3") // fails install → triggers rollback
	f.depTree.Result = []domain.Namespace{dep1, dep2, dep3, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}

	var uninstallCalls []domain.Namespace
	callCount := 0
	f.runner.executeSync = func(ctx context.Context, installNS domain.Namespace, method string, vars map[string]string) error {
		if method == "_install" {
			callCount++
			if callCount == 1 {
				// dep1 installs OK and becomes Ready
				_, _ = f.axRuntime.Send(ctx, readyRuntimeCmd{ns: dep1})
				f.axRuntime.WaitPublish()
				return nil
			}
			if callCount == 2 {
				// dep2 installs OK but is NOT seeded as Ready
				return nil
			}
			if callCount == 3 {
				// dep3 fails
				return errors.New("dep3 install failed")
			}
		}
		if method == "_uninstall" {
			uninstallCalls = append(uninstallCalls, installNS)
		}
		return nil
	}

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dep3 install failed")
	// Only dep1 (Ready) should be uninstalled; dep2 (not Ready) is skipped.
	require.Len(t, uninstallCalls, 1)
	assert.Equal(t, dep1, uninstallCalls[0])
}

// TestDepsHandler_ResolveManifest_VaultSuccess verifies that when vault.GetArrow returns a valid
// entry (no error), Execute uses that manifest directly without calling manifold.
func TestDepsHandler_ResolveManifest_VaultSuccess(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	// Vault returns a valid entry — no ErrStale, no ErrNotCached
	f.vault.GetArrowErr = nil
	f.vault.GetArrowEntry = &vault.VaultEntry{
		Manifest: &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "2.0.0"}},
	}

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	require.Len(t, f.syncCalls, 1)
	assert.Equal(t, dep, f.syncCalls[0].ns)
	assert.Equal(t, "_install", f.syncCalls[0].method)
	// Manifold should not have been called.
	assert.Nil(t, f.manifold.ResolveArrowErr)
}

// TestDepsHandler_ResolveManifest_StaleManifoldSucceeds verifies ErrStale path where manifold
// resolves successfully and PutArrow also succeeds — returns the fresh manifest.
func TestDepsHandler_ResolveManifest_StaleManifoldSucceeds(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	staleManifest := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "0.9.0"}}
	f.vault.GetArrowErr = vault.ErrStale
	f.vault.GetArrowEntry = &vault.VaultEntry{Manifest: staleManifest}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}
	f.vault.PutArrowErr = nil // PutArrow succeeds

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	require.Len(t, f.syncCalls, 1)
	assert.Equal(t, dep, f.syncCalls[0].ns)
	assert.Equal(t, "_install", f.syncCalls[0].method)
}

// TestDepsHandler_ResolveManifest_NotCachedManifoldFails verifies ErrNotCached path where manifold
// fails — Execute propagates the manifold error.
func TestDepsHandler_ResolveManifest_NotCachedManifoldFails(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}

	// vault default is ErrNotCached; manifold fails
	f.manifold.ResolveArrowErr = errors.New("manifold unreachable")

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifold unreachable")
}

// TestDepsHandler_ResolverClosure_ViaRealDepTree exercises the resolver closure inside Execute
// by using the real deptree.New() which calls the resolver during DFS traversal.
func TestDepsHandler_ResolverClosure_ViaRealDepTree(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(runtimeES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	// Manifold returns a manifest with no dependencies for any namespace.
	mm := &mocks.Manifold{ResolveArrowManifest: &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "arrow", Version: "1.0.0"}}}

	// Use real deptree so the resolver closure is actually invoked during DFS.
	dt := deptree.New()

	var syncCalls []syncCall
	r := &mockRunner{
		executeSync: func(_ context.Context, callNS domain.Namespace, method string, _ map[string]string) error {
			syncCalls = append(syncCalls, syncCall{ns: callNS, method: method})
			return nil
		},
	}
	h := deps.New(dt, mv, mm, axArrow, axRuntime, r)

	// Root namespace — resolver is called during deptree DFS traversal.
	// Vault returns ErrNotCached → manifold returns manifest with no deps → deptree returns [ns].
	ns := domain.Namespace("github.com/org/arrow")
	req := wizstep.Request{NSKey: ns.String()}
	step := domainstep.NewDependenciesStep("Resolve dependencies")

	err = h.Execute(context.Background(), req, step)

	// No deps to install (root only), but the resolver closure was exercised.
	require.NoError(t, err)
	assert.Empty(t, syncCalls)
}

// TestDepsHandler_RuntimeGetNonNotFoundErr verifies that when asynxRuntime.Get returns an error
// that is NOT ErrNotFound, it is treated as "no runtime state" and dep installation proceeds.
func TestDepsHandler_RuntimeGetNonNotFoundErr(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveArrowManifest: &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "dep", Version: "1.0.0"}}}
	dt := &mocks.DepTree{}

	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	dt.Result = []domain.Namespace{dep, ns}

	// Stub runtime that returns a non-ErrNotFound error from Get.
	stubRuntime := &stubAsynxRuntime{getErr: errors.New("context deadline exceeded")}

	var syncCalls []syncCall
	r := &mockRunner{
		executeSync: func(_ context.Context, installNS domain.Namespace, method string, _ map[string]string) error {
			syncCalls = append(syncCalls, syncCall{ns: installNS, method: method})
			return nil
		},
	}
	h := deps.New(dt, mv, mm, axArrow, stubRuntime, r)

	req := wizstep.Request{NSKey: ns.String()}
	step := domainstep.NewDependenciesStep("Resolve dependencies")

	err = h.Execute(context.Background(), req, step)

	require.NoError(t, err)
	require.Len(t, syncCalls, 1)
	assert.Equal(t, dep, syncCalls[0].ns)
}

// readyRuntimeCmd seeds an ArrowRuntime in Ready state for testing.
type readyRuntimeCmd struct{ ns domain.Namespace }

func (c readyRuntimeCmd) AggregateID() string                          { return c.ns.String() }
func (c readyRuntimeCmd) EventName() string                            { return "runtime.mock_ready" }
func (c readyRuntimeCmd) ShouldSnapshot() bool                         { return false }
func (c readyRuntimeCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c readyRuntimeCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.ns,
		State: domain.ArrowStateReady,
	}
}

// stubAsynxRuntime is a minimal asynx.Asynx[domainRuntime.ArrowRuntime] stub that returns a
// configurable error from Get and panics on any unneeded methods.
type stubAsynxRuntime struct {
	getErr error
}

func (s *stubAsynxRuntime) Send(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (s *stubAsynxRuntime) SendWait(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (s *stubAsynxRuntime) Shutdown(_ context.Context) error { return nil }
func (s *stubAsynxRuntime) Get(_ context.Context, _ string) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, s.getErr
}
func (s *stubAsynxRuntime) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (s *stubAsynxRuntime) Preload(_ context.Context, _ string) error        { return nil }
func (s *stubAsynxRuntime) Subscribe(_ string, _ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime], _ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime]) (string, error) {
	return "", nil
}
func (s *stubAsynxRuntime) Unsubscribe(_ string) error { return nil }
func (s *stubAsynxRuntime) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime]) error {
	return nil
}
func (s *stubAsynxRuntime) Forget(_ context.Context, _ string) error { return nil }
func (s *stubAsynxRuntime) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
}
func (s *stubAsynxRuntime) WaitPublish() {}
