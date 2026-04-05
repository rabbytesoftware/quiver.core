package deps_test

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/deps"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// depFixture wires a DependenciesHandler with real asynx stores and mock engines.
type depFixture struct {
	handler      deps.DependenciesHandler
	axArrow      asynx.Asynx[domain.Arrow]
	axRuntime    asynx.Asynx[domainRuntime.ArrowRuntime]
	vault        *mocks.Vault
	manifold     *mocks.Manifold
	depTree      *mocks.DepTree
	syncCalls    []syncCall
	syncErr      error
}

type syncCall struct {
	ns     domain.Namespace
	method string
}

func newDepFixture(t *testing.T) *depFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().WithEventStore(arrowES).Build()
	require.NoError(t, err)
	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().WithEventStore(runtimeES).Build()
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

	h := deps.New(dt, mv, mm, axArrow, axRuntime)
	h.SetSyncInstall(func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
		f.syncCalls = append(f.syncCalls, syncCall{ns: ns, method: method})
		return f.syncErr
	})
	f.handler = h

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
	require.NoError(t, f.axRuntime.Send(context.Background(), readyRuntimeCmd{ns: dep}))
	f.axRuntime.WaitPublish()

	err := f.handler.Execute(context.Background(), f.req(ns), f.step())

	require.NoError(t, err)
	assert.Empty(t, f.syncCalls, "should not install already-ready dep")
}

// TestDepsHandler_DepNotInstalled_InstallsCalled verifies uninstalled deps are installed.
func TestDepsHandler_DepNotInstalled_InstallsCalled(t *testing.T) {
	f := newDepFixture(t)
	ns := domain.Namespace("github.com/org/arrow")
	dep := domain.Namespace("github.com/org/dep")
	f.depTree.Result = []domain.Namespace{dep, ns}
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{Name: "dep", Version: "1.0.0"}

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
	f.manifold.ResolveArrowManifest = &domain.ArrowManifest{Name: "dep", Version: "1.0.0"}

	callN := 0
	f.handler.SetSyncInstall(func(ctx context.Context, installNS domain.Namespace, method string, vars map[string]string) error {
		callN++
		if method == "_install" && callN == 1 {
			// dep1 installs successfully — seed it as Ready for rollback check
			_ = f.axRuntime.Send(ctx, readyRuntimeCmd{ns: dep1})
			f.axRuntime.WaitPublish()
			return nil
		}
		if method == "_install" && callN == 2 {
			return errors.New("dep2 install failed")
		}
		return nil // uninstall calls
	})

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

// readyRuntimeCmd seeds an ArrowRuntime in Ready state for testing.
type readyRuntimeCmd struct{ ns domain.Namespace }

func (c readyRuntimeCmd) AggregateID() string { return c.ns.String() }
func (c readyRuntimeCmd) EventName() string   { return "runtime.mock_ready" }
func (c readyRuntimeCmd) ShouldSnapshot() bool { return false }
func (c readyRuntimeCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c readyRuntimeCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Namespace: c.ns,
		State:     domain.ArrowStateReady,
	}
}
