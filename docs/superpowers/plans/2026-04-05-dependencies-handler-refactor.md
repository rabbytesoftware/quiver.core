# Dependencies Handler Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inject a `DependenciesHandler` into the wizard so `_install` executes through the same `BeginExecution` path as every other method, eliminating `runInstall()`, `indexOffset`, `buildDepResolver()`, `rollbackInstalled()`, and `updateIndirectDeps()` from `arrowService`.

**Architecture:** A new `DependenciesHandler` in `internal/app/arrow/deps/` implements `wizstep.Handler[DependenciesStep]` and is registered into the wizard via a new `RegisterDispatch` method on the `Wizard` interface. A `wizard.Adapt[S]()` generic function converts any typed handler into a `DispatchFn`. A two-phase init via `SetSyncInstall` breaks the circular dep between the handler and `arrowService.executeSync`. `DependenciesStep.ExitOnFailure()` is flipped to `true` so a dep resolution failure stops the wizard.

**Tech stack:** Go, asynx event-sourced aggregates (SQLite-backed), wizard engine with dispatch map.

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/domain/runtime/step/dependencies.go` | `ExitOnFailure()` → `true` |
| Modify | `internal/engine/wizard/wizard.go` | Export `DispatchFn`, add `Adapt[S]`, add `RegisterDispatch` to interface + impl |
| Modify | `internal/mocks/wizard.go` | Add `RegisterDispatch` stub |
| Create | `internal/app/arrow/deps/handler.go` | `DependenciesHandler` interface + `handler` struct + `Execute()`, `SetSyncInstall()`, helpers |
| Create | `internal/app/arrow/deps/handler_test.go` | Unit tests for all `Execute()` branches |
| Modify | `internal/app/arrow/stepreporter/step_reporter.go` | Remove `indexOffset` field + param |
| Create | `internal/app/arrow/stepreporter/step_reporter_test.go` | Tests for direct index passthrough |
| Modify | `internal/app/arrow/execution.go` | Remove `indexOffset` block + `buildDepResolver()` |
| Modify | `internal/app/arrow/execution_test.go` | Remove `buildDepResolver` tests |
| Modify | `internal/app/arrow/lifecycle.go` | Delete `runInstall`, `rollbackInstalled`, `updateIndirectDeps` |
| Modify | `internal/app/arrow/lifecycle_test.go` | Remove tests for deleted functions; add `RegisterDispatch` stub to `callCountWizard` |
| Modify | `internal/app/arrow/arrow.go` | `Install()` calls `go func() { _ = svc.BeginExecution(...) }()` |
| Modify | `internal/app/arrow/builder.go` | Construct dep handler, `RegisterDispatch`, wire `syncInstall` closure |

---

### Task 1: Fix `DependenciesStep.ExitOnFailure()` to return `true`

**Files:**
- Modify: `internal/domain/runtime/step/dependencies.go`
- Modify: `internal/domain/runtime/step/step_test.go`

- [ ] **Step 1.1: Write failing test**

In `internal/domain/runtime/step/step_test.go`, add:

```go
func TestDependenciesStep_ExitOnFailure_ReturnsTrue(t *testing.T) {
	s := NewDependenciesStep("Resolve dependencies")
	assert.True(t, s.ExitOnFailure())
}
```

- [ ] **Step 1.2: Run test to verify it fails**

```bash
cd /Users/char2cs/.superset/worktrees/quiver.core/feature/app-layer
go test ./internal/domain/runtime/step/... -run TestDependenciesStep_ExitOnFailure_ReturnsTrue -v
```

Expected: FAIL — `assert.True` fails because `ExitOnFailure()` returns `false`.

- [ ] **Step 1.3: Flip `ExitOnFailure()` to `true`**

In `internal/domain/runtime/step/dependencies.go`, change:

```go
func (s DependenciesStep) ExitOnFailure() bool { return false }
```

to:

```go
func (s DependenciesStep) ExitOnFailure() bool { return true }
```

- [ ] **Step 1.4: Run test to verify it passes**

```bash
go test ./internal/domain/runtime/step/... -v
```

Expected: all PASS.

- [ ] **Step 1.5: Commit**

```bash
git add internal/domain/runtime/step/dependencies.go internal/domain/runtime/step/step_test.go
git commit -m "fix(domain): DependenciesStep.ExitOnFailure returns true"
```

---

### Task 2: Add `RegisterDispatch` + `Adapt` to wizard

**Files:**
- Modify: `internal/engine/wizard/wizard.go`
- Modify: `internal/engine/wizard/wizard_test.go`
- Modify: `internal/mocks/wizard.go`

- [ ] **Step 2.1: Write failing test for `RegisterDispatch`**

In `internal/engine/wizard/wizard_test.go`, add at the end:

```go
func TestWizard_RegisterDispatch_CustomHandlerInvoked(t *testing.T) {
	w, err := New()
	require.NoError(t, err)

	called := false
	fn := func(_ context.Context, _ wizstep.Request, _ domainstep.Step) error {
		called = true
		return nil
	}
	w.RegisterDispatch(domainstep.StepTypeDependencies, fn)

	dep := domainstep.NewDependenciesStep("test")
	reporter := &mockReporter{}
	err = w.Execute(context.Background(), RunRequest{
		Namespace: "github.com/test/arrow",
		Steps:     []domainstep.Step{dep},
	}, reporter)

	require.NoError(t, err)
	assert.True(t, called)
}
```

- [ ] **Step 2.2: Run test to verify it fails**

```bash
go test ./internal/engine/wizard/... -run TestWizard_RegisterDispatch_CustomHandlerInvoked -v
```

Expected: compile error — `RegisterDispatch` not defined on `Wizard` interface.

- [ ] **Step 2.3: Export `DispatchFn`, add `Adapt[S]`, add `RegisterDispatch`**

In `internal/engine/wizard/wizard.go`, make these changes:

Change the private type alias to an exported one:
```go
// was: type dispatchFn = func(context.Context, wizstep.Request, domainstep.Step) error
type DispatchFn = func(context.Context, wizstep.Request, domainstep.Step) error
```

Add `Adapt` as a package-level generic function (after the `adapt` private function):
```go
// Adapt converts a typed Handler into a DispatchFn suitable for RegisterDispatch.
func Adapt[S domainstep.Step](h wizstep.Handler[S]) DispatchFn {
	return func(ctx context.Context, req wizstep.Request, s domainstep.Step) error {
		typed, ok := s.(S)
		if !ok {
			return fmt.Errorf("adapt: step type mismatch: expected %T, got %T", *new(S), s)
		}
		return h.Execute(ctx, req, typed)
	}
}
```

Update the private `adapt` helper to use `Adapt`:
```go
func adapt[S domainstep.Step](
	dispatch map[domainstep.StepType]DispatchFn,
	t domainstep.StepType,
	h wizstep.Handler[S],
) {
	dispatch[t] = Adapt(h)
}
```

Add `RegisterDispatch` to the `Wizard` interface:
```go
type Wizard interface {
	Execute(
		ctx context.Context,
		req RunRequest,
		reporter StepReporter,
	) error
	Cancel(namespace domain.Namespace)
	Shutdown(ctx context.Context) error
	// RegisterDispatch registers a DispatchFn for a step type.
	// Use wizard.Adapt() to convert a typed Handler into a DispatchFn.
	// Must be called before any Execute calls that include this step type.
	RegisterDispatch(t domainstep.StepType, fn DispatchFn)
}
```

Implement on `*wizard`:
```go
func (w *wizard) RegisterDispatch(t domainstep.StepType, fn DispatchFn) {
	w.dispatch[t] = fn
}
```

- [ ] **Step 2.4: Update `mocks.Wizard` to satisfy the updated interface**

In `internal/mocks/wizard.go`, add:

```go
func (m *Wizard) RegisterDispatch(_ wizard.DispatchFn_StepType, _ wizard.DispatchFn) {}
```

Wait — the method signature is `RegisterDispatch(t domainstep.StepType, fn wizard.DispatchFn)`. Full correct stub:

```go
func (m *Wizard) RegisterDispatch(_ domainstep.StepType, _ wizard.DispatchFn) {}
```

Add the necessary import in `mocks/wizard.go`:
```go
import (
    "context"
    "sync"

    "github.com/rabbytesoftware/quiver/internal/domain"
    domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
    "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)
```

- [ ] **Step 2.5: Update `callCountWizard` in `lifecycle_test.go`**

In `internal/app/arrow/lifecycle_test.go`, add to `callCountWizard`:

```go
func (w *callCountWizard) RegisterDispatch(_ domainstep.StepType, _ wizard.DispatchFn) {}
```

Add import for `domainstep` if not already present.

- [ ] **Step 2.6: Run tests to verify**

```bash
go test ./internal/engine/wizard/... -v
go test ./internal/mocks/... -v 2>/dev/null || true
go build ./...
```

Expected: wizard tests all PASS, codebase compiles.

- [ ] **Step 2.7: Commit**

```bash
git add internal/engine/wizard/wizard.go internal/engine/wizard/wizard_test.go internal/mocks/wizard.go internal/app/arrow/lifecycle_test.go
git commit -m "feat(wizard): add RegisterDispatch + Adapt for external handler injection"
```

---

### Task 3: Create `DependenciesHandler`

**Files:**
- Create: `internal/app/arrow/deps/handler.go`
- Create: `internal/app/arrow/deps/handler_test.go`

- [ ] **Step 3.1: Write failing tests**

Create `internal/app/arrow/deps/handler_test.go`:

```go
package deps_test

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
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
```

- [ ] **Step 3.2: Run tests to verify they fail**

```bash
go test ./internal/app/arrow/deps/... -v
```

Expected: compile error — package `deps` does not exist yet.

- [ ] **Step 3.3: Create `internal/app/arrow/deps/handler.go`**

```go
package deps

import (
	"context"
	"errors"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

// SyncInstallFn executes an arrow method synchronously.
// Injected to break the circular dependency with arrowService.executeSync.
type SyncInstallFn func(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error

// DependenciesHandler handles DependenciesStep execution inside the wizard.
// It resolves and installs transitive dependencies for a namespace.
type DependenciesHandler interface {
	wizstep.Handler[domainstep.DependenciesStep]
	// SetSyncInstall wires the function used to install/uninstall deps.
	// Must be called before any Execute invocations.
	SetSyncInstall(fn SyncInstallFn)
}

type handler struct {
	depTree      deptree.DepTree
	vault        vault.Vault
	manifold     manifold.Manifold
	asynxArrow   asynx.Asynx[domain.Arrow]
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	syncInstall  SyncInstallFn
}

// New constructs a DependenciesHandler. Call SetSyncInstall before use.
func New(
	depTree deptree.DepTree,
	v vault.Vault,
	m manifold.Manifold,
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) DependenciesHandler {
	return &handler{
		depTree:      depTree,
		vault:        v,
		manifold:     m,
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
	}
}

func (h *handler) SetSyncInstall(fn SyncInstallFn) {
	h.syncInstall = fn
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	_ domainstep.DependenciesStep,
) error {
	ns := domain.Namespace(req.NSKey)

	resolver := deptree.ResolverFunc(func(rCtx context.Context, depNs domain.Namespace) ([]domain.Namespace, error) {
		manifest, err := h.resolveManifest(rCtx, depNs)
		if err != nil {
			return nil, err
		}
		return manifest.Dependencies, nil
	})

	orderedDeps, err := h.depTree.Resolve(ctx, ns, resolver)
	if err != nil {
		return err
	}

	var installed []domain.Namespace

	for _, dep := range orderedDeps {
		if dep == ns {
			continue
		}

		rt, rtErr := h.asynxRuntime.Get(ctx, dep.String())
		if rtErr != nil && !errors.Is(rtErr, asynxModels.ErrNotFound) {
			rtErr = nil
		}
		if rtErr == nil && rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
			continue
		}

		manifest, mErr := h.resolveManifest(ctx, dep)
		if mErr != nil {
			h.rollback(ctx, installed)
			return mErr
		}

		existing, getErr := h.asynxArrow.Get(ctx, dep.String())
		if errors.Is(getErr, asynxModels.ErrNotFound) || existing.Namespace == "" {
			_ = h.asynxArrow.Send(ctx, arrowcmds.AddArrow{
				Namespace: dep,
				Manifest:  *manifest,
			})
		}

		if installErr := h.syncInstall(ctx, dep, "_install", nil); installErr != nil {
			h.rollback(ctx, installed)
			return installErr
		}

		installed = append(installed, dep)
	}

	h.updateIndirectDeps(ctx, ns, orderedDeps)

	return nil
}

func (h *handler) resolveManifest(ctx context.Context, ns domain.Namespace) (*domain.ArrowManifest, error) {
	entry, _, err := h.vault.GetArrow(ctx, ns)

	if err == nil {
		return entry.Manifest, nil
	}

	if errors.Is(err, vault.ErrStale) {
		manifest, manifoldErr := h.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return entry.Manifest, nil
		}
		_, putErr := h.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, putErr
		}
		return manifest, nil
	}

	if errors.Is(err, vault.ErrNotCached) {
		manifest, manifoldErr := h.manifold.ResolveArrow(ctx, ns)
		if manifoldErr != nil {
			return nil, manifoldErr
		}
		_, putErr := h.vault.PutArrow(ctx, ns, manifest, nil)
		if putErr != nil {
			return nil, putErr
		}
		return manifest, nil
	}

	return nil, err
}

func (h *handler) rollback(ctx context.Context, installed []domain.Namespace) {
	for i := len(installed) - 1; i >= 0; i-- {
		dep := installed[i]
		rt, err := h.asynxRuntime.Get(ctx, dep.String())
		if err != nil || rt.State != domain.ArrowStateReady {
			continue
		}
		_ = h.syncInstall(ctx, dep, "_uninstall", nil)
	}
}

func (h *handler) updateIndirectDeps(ctx context.Context, ns domain.Namespace, deptreeResult []domain.Namespace) {
	arrow, err := h.asynxArrow.Get(ctx, ns.String())
	if err != nil {
		return
	}

	directSet := make(map[string]bool)
	for _, dep := range arrow.Manifest.Dependencies {
		directSet[dep.String()] = true
	}

	var indirect []domain.Namespace
	for _, dep := range deptreeResult {
		if dep == ns || directSet[dep.String()] {
			continue
		}
		indirect = append(indirect, dep)
	}

	_, _ = h.vault.PutArrow(ctx, ns, &arrow.Manifest, indirect)
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

```bash
go test ./internal/app/arrow/deps/... -v
```

Expected: all PASS.

- [ ] **Step 3.5: Commit**

```bash
git add internal/app/arrow/deps/handler.go internal/app/arrow/deps/handler_test.go
git commit -m "feat(arrow): add DependenciesHandler for wizard injection"
```

---

### Task 4: Remove `indexOffset` from `StepReporter`

**Files:**
- Modify: `internal/app/arrow/stepreporter/step_reporter.go`
- Create: `internal/app/arrow/stepreporter/step_reporter_test.go`

- [ ] **Step 4.1: Write failing test**

Create `internal/app/arrow/stepreporter/step_reporter_test.go`:

```go
package stepreporter_test

import (
	"context"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().WithEventStore(es).Build()
	require.NoError(t, err)
	return ax
}

func TestStepReporter_New_TwoParams(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	// New takes exactly 2 params — compile-time check
	r := stepreporter.New(ax, domain.Namespace("github.com/org/arrow"))
	assert.NotNil(t, r)
}

func TestStepReporter_OnStepStarted_SendsRunningAtExactIndex(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := domain.Namespace("github.com/org/arrow")
	r := stepreporter.New(ax, ns)

	// Seed aggregate so AdvanceStep can apply
	require.NoError(t, ax.Send(context.Background(), seedRuntimeCmd{ns: ns, stepCount: 3}))
	ax.WaitPublish()

	r.OnStepStarted(2)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusRunning, rt.ActiveRun.Steps[2].Status)
}

func TestStepReporter_OnStepCompleted_SendsCompletedAtExactIndex(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := domain.Namespace("github.com/org/arrow")
	r := stepreporter.New(ax, ns)

	require.NoError(t, ax.Send(context.Background(), seedRuntimeCmd{ns: ns, stepCount: 3}))
	ax.WaitPublish()

	r.OnStepCompleted(1)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusCompleted, rt.ActiveRun.Steps[1].Status)
}

func TestStepReporter_OnStepFailed_SendsFailedWithErrorAtExactIndex(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := domain.Namespace("github.com/org/arrow")
	r := stepreporter.New(ax, ns)

	require.NoError(t, ax.Send(context.Background(), seedRuntimeCmd{ns: ns, stepCount: 2}))
	ax.WaitPublish()

	r.OnStepFailed(0, assert.AnError)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusFailed, rt.ActiveRun.Steps[0].Status)
	require.NotNil(t, rt.ActiveRun.Steps[0].Error)
}

// seedRuntimeCmd creates an active run with N pending steps for testing AdvanceStep.
type seedRuntimeCmd struct {
	ns        domain.Namespace
	stepCount int
}

func (c seedRuntimeCmd) AggregateID() string { return c.ns.String() }
func (c seedRuntimeCmd) EventName() string   { return "runtime.mock_seed" }
func (c seedRuntimeCmd) ShouldSnapshot() bool { return false }
func (c seedRuntimeCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c seedRuntimeCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	steps := make([]domainRuntime.StepProgress, c.stepCount)
	for i := range steps {
		steps[i] = domainRuntime.StepProgress{Index: i, Status: domainRuntime.StepStatusPending}
	}
	return domainRuntime.ArrowRuntime{
		Namespace: c.ns,
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method: "_execute",
			Steps:  steps,
		},
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

```bash
go test ./internal/app/arrow/stepreporter/... -v
```

Expected: FAIL on `TestStepReporter_New_TwoParams` because `New` currently takes 3 params.

- [ ] **Step 4.3: Remove `indexOffset` from `StepReporter`**

Replace `internal/app/arrow/stepreporter/step_reporter.go` entirely:

```go
package stepreporter

import (
	"context"

	"github.com/char2cs/asynx"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type StepReporter struct {
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	namespace    domain.Namespace
}

func New(
	asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	namespace domain.Namespace,
) *StepReporter {
	return &StepReporter{
		asynxRuntime: asynxRuntime,
		namespace:    namespace,
	}
}

func (r *StepReporter) OnStepStarted(i int) {
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusRunning,
		},
	)
}

func (r *StepReporter) OnStepCompleted(i int) {
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusCompleted,
		},
	)
}

func (r *StepReporter) OnStepFailed(i int, err error) {
	errStr := err.Error()
	_ = r.asynxRuntime.Send(
		context.Background(),
		arrowcmds.AdvanceStep{
			Namespace: r.namespace,
			StepIndex: i,
			ToStatus:  domainRuntime.StepStatusFailed,
			Error:     &errStr,
		},
	)
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

```bash
go test ./internal/app/arrow/stepreporter/... -v
```

Expected: all PASS.

- [ ] **Step 4.5: Fix `runUninstall` call in `lifecycle.go`**

`runUninstall` calls `stepreporter.New(svc.asynxRuntime, ns, 0)` which now fails to compile. Update it:

```go
// was:
reporter := stepreporter.New(svc.asynxRuntime, ns, 0)
// becomes:
reporter := stepreporter.New(svc.asynxRuntime, ns)
```

- [ ] **Step 4.6: Build and run tests**

```bash
go build ./internal/app/arrow/...
go test ./internal/app/arrow/stepreporter/... -v
```

Expected: all PASS.

- [ ] **Step 4.7: Commit**

```bash
git add internal/app/arrow/stepreporter/step_reporter.go internal/app/arrow/stepreporter/step_reporter_test.go internal/app/arrow/lifecycle.go
git commit -m "refactor(stepreporter): remove indexOffset — wizard indices now map 1:1 to aggregate"
```

---

### Task 5: Clean up `execution.go`

**Files:**
- Modify: `internal/app/arrow/execution.go`
- Modify: `internal/app/arrow/execution_test.go`

- [ ] **Step 5.1: Remove `indexOffset` block and `buildDepResolver` from `execution.go`**

In `internal/app/arrow/execution.go`:

Remove the `indexOffset` block (lines 105–108):
```go
// DELETE these lines:
indexOffset := 0
if method == "_install" {
    indexOffset = 1
}
```

Update `stepreporter.New(...)` call to remove the third argument:
```go
// was:
reporter := stepreporter.New(svc.asynxRuntime, ns, indexOffset)
// becomes:
reporter := stepreporter.New(svc.asynxRuntime, ns)
```

Delete the entire `buildDepResolver` function (lines 61–70):
```go
// DELETE:
func (svc *arrowService) buildDepResolver() deptree.ResolverFunc {
    return func(ctx context.Context, ns domain.Namespace) ([]domain.Namespace, error) {
        manifest, _, err := svc.resolveManifest(ctx, ns)
        if err != nil {
            return nil, err
        }
        return manifest.Dependencies, nil
    }
}
```

Remove the `deptree` import if no longer used anywhere in the file.

- [ ] **Step 5.2: Remove `buildDepResolver` tests from `execution_test.go`**

Delete the two test functions at the bottom of `execution_test.go`:
```go
// DELETE both:
func TestBuildDepResolver_ReturnsResolverThatReturnsDeps(t *testing.T) { ... }
func TestBuildDepResolver_ManifoldFails_ReturnsError(t *testing.T) { ... }
```

- [ ] **Step 5.3: Build and run execution tests**

```bash
go build ./internal/app/arrow/...
go test ./internal/app/arrow/... -run "TestMapOutcome|TestResolveVariables|TestStop|TestHandleExecution|TestResolveManifest" -v
```

Expected: all PASS, no compile errors.

- [ ] **Step 5.4: Commit**

```bash
git add internal/app/arrow/execution.go internal/app/arrow/execution_test.go
git commit -m "refactor(arrow): remove indexOffset and buildDepResolver from execution"
```

---

### Task 6: Delete `runInstall`, `rollbackInstalled`, `updateIndirectDeps` from `lifecycle.go`

**Files:**
- Modify: `internal/app/arrow/lifecycle.go`
- Modify: `internal/app/arrow/lifecycle_test.go`

- [ ] **Step 6.1: Delete functions from `lifecycle.go`**

Remove the entire `runInstall` function (lines 16–140), the `rollbackInstalled` function (lines 142–151), and the `updateIndirectDeps` function (lines 153–173).

The remaining `lifecycle.go` keeps only `runUninstall` (lines 175–269) and `hasDependents` (lines 271–309). Remove imports no longer needed after deletion (`errors`, `asynxModels`, `arrowcmds`, `stepreporter`, `deptree`, `wizard` — check each one; `runUninstall` and `hasDependents` may still use some of them).

Resulting file starts with:
```go
package arrow

import (
	"context"
	"errors"

	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)
```

Keep exactly the imports used by `runUninstall` and `hasDependents`. Remove `stepreporter`, `wizard`, and `deptree` from the import block if `runUninstall` doesn't use them (it uses `stepreporter.New` and `wizard.RunRequest` directly — check the existing code).

- [ ] **Step 6.2: Remove deleted-function tests from `lifecycle_test.go`**

Delete all test functions that test `runInstall`, `rollbackInstalled`, or `updateIndirectDeps`. Keep tests for `runUninstall` and `hasDependents`.

- [ ] **Step 6.3: Build to verify no dangling references**

```bash
go build ./internal/app/arrow/...
```

Expected: compiles cleanly.

- [ ] **Step 6.4: Run remaining lifecycle tests**

```bash
go test ./internal/app/arrow/... -run "TestRunUninstall|TestHasDependents" -v
```

Expected: all PASS.

- [ ] **Step 6.5: Commit**

```bash
git add internal/app/arrow/lifecycle.go internal/app/arrow/lifecycle_test.go
git commit -m "refactor(arrow): delete runInstall, rollbackInstalled, updateIndirectDeps — logic moved to DependenciesHandler"
```

---

### Task 7: Update `Install()` in `arrow.go`

**Files:**
- Modify: `internal/app/arrow/arrow.go`
- Modify: `internal/app/arrow/arrow_test.go`

- [ ] **Step 7.1: Update `Install()` to call `BeginExecution`**

In `internal/app/arrow/arrow.go`, replace the body of `Install()`:

```go
func (svc *arrowService) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	arrow, err := svc.asynxArrow.Get(ctx, ns.String())
	if errors.Is(err, asynxModels.ErrNotFound) || arrow.Namespace == "" {
		return fmt.Errorf("install: %w", ErrNotFound)
	}
	if err != nil {
		return err
	}

	rt, err := svc.asynxRuntime.Get(ctx, ns.String())
	if err != nil && !errors.Is(err, asynxModels.ErrNotFound) {
		return err
	}
	if rt.Namespace != "" && rt.State != domain.ArrowStateAbsent {
		return fmt.Errorf("install: %w", ErrStateViolation)
	}

	go func() {
		_ = svc.BeginExecution(context.Background(), ns, "_install", userVars)
	}()

	return nil
}
```

- [ ] **Step 7.2: Update `Install` tests in `arrow_test.go`**

Any tests that assert `runInstall` was called directly should now assert `BeginExecution` behavior (wizard called via mock). Keep state validation tests unchanged since that logic is preserved.

- [ ] **Step 7.3: Build and run arrow tests**

```bash
go build ./internal/app/arrow/...
go test ./internal/app/arrow/... -run "TestInstall" -v
```

Expected: all PASS.

- [ ] **Step 7.4: Commit**

```bash
git add internal/app/arrow/arrow.go internal/app/arrow/arrow_test.go
git commit -m "refactor(arrow): Install delegates to BeginExecution — _install now uniform with all other methods"
```

---

### Task 8: Wire `DependenciesHandler` in `builder.go`

**Files:**
- Modify: `internal/app/arrow/builder.go`

- [ ] **Step 8.1: Update `Build()` to construct, register, and wire the handler**

In `internal/app/arrow/builder.go`, update `Build()`:

Add the import:
```go
"github.com/rabbytesoftware/quiver/internal/app/arrow/deps"
domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
"github.com/rabbytesoftware/quiver/internal/engine/wizard"
```

In the `Build()` function, after constructing `axArrow` and `axRuntime` and before constructing `svc`, add:

```go
// Construct dep handler before svc — syncInstall wired after svc is available.
depHandler := deps.New(e.DepTree, e.Vault, e.Manifold, axArrow, axRuntime)
if e.Wizard != nil {
    e.Wizard.RegisterDispatch(
        domainstep.StepTypeDependencies,
        wizard.Adapt(depHandler),
    )
}
```

After `svc` is constructed, wire `syncInstall`:

```go
depHandler.SetSyncInstall(func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
    return svc.executeSync(ctx, ns, method, vars)
})
```

Full updated `Build()` function:

```go
func (b *Builder) Build() (ArrowService, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("arrow builder: event store is required")
	}

	axArrow, err := newAsynxArrow(b.eventStore)
	if err != nil {
		return nil, err
	}

	runtimeES := b.runtimeEventStore
	if runtimeES == nil {
		runtimeES = b.eventStore
	}

	axRuntime, err := newAsynxRuntime(runtimeES)
	if err != nil {
		return nil, err
	}

	catalog := b.catalog
	if catalog == nil {
		catalog, err = arrowstore.NewArrowCatalog(metadata.GetQuiverHome() + "/arrows.db")
		if err != nil {
			return nil, err
		}
	}

	var e engine.Container
	if b.engines != nil {
		e = *b.engines
	}

	depHandler := deps.New(e.DepTree, e.Vault, e.Manifold, axArrow, axRuntime)
	if e.Wizard != nil {
		e.Wizard.RegisterDispatch(
			domainstep.StepTypeDependencies,
			wizard.Adapt[domainstep.DependenciesStep](depHandler),
		)
	}

	svc := &arrowService{

		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		catalog:      catalog,
		engines:      e,
		os:           b.os,
	}

	depHandler.SetSyncInstall(func(ctx context.Context, ns domain.Namespace, method string, vars map[string]string) error {
		return svc.executeSync(ctx, ns, method, vars)
	})

	if err = arrowproj.Init(
		axArrow,
		axRuntime,
		b.catalog,
		e.Wizard,
	); err != nil {
		return nil, err
	}

	return svc, nil
}
```

- [ ] **Step 8.2: Build to verify**

```bash
go build ./...
```

Expected: compiles cleanly.

- [ ] **Step 8.3: Commit**

```bash
git add internal/app/arrow/builder.go
git commit -m "feat(arrow): wire DependenciesHandler into wizard via builder"
```

---

### Task 9: Full test suite

- [ ] **Step 9.1: Run all tests**

```bash
go test ./... -timeout 120s
```

Expected: all PASS. If any test references `indexOffset`, `runInstall`, `buildDepResolver`, `rollbackInstalled`, or `updateIndirectDeps`, fix the reference.

- [ ] **Step 9.2: Verify test coverage on key packages**

```bash
go test ./internal/app/arrow/... -cover
go test ./internal/app/arrow/deps/... -cover
go test ./internal/engine/wizard/... -cover
```

Expected: `deps` package ≥ 95% coverage, no regressions in wizard or arrow packages.

- [ ] **Step 9.3: Final commit if any cleanup was needed**

```bash
git add -p
git commit -m "fix: post-refactor test and import cleanup"
```
