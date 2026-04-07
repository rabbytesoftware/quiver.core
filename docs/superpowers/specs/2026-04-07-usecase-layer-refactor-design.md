# Use-Case Layer Refactor — Arrow & Quiver

**Date:** 2026-04-07
**Scope:** `internal/app/arrow/` and `internal/app/quiver/`

---

## Motivation

The current use-case layer has three structural problems:

1. **God struct.** `arrowService` accumulates every operation — CRUD, queries, execution, lifecycle, dependency resolution — into one struct spread across multiple files with no clear ownership boundary.

2. **Artificial projections package.** `arrow/projections/` is a grab-bag of event handlers with no unifying concept. It exists only to avoid circular imports.

3. **Circular dependency with two-phase init hacks.** `WizardExecutor` (inside `projections/`) needs to call back into `arrowService` (inside `arrow/`), creating `arrow → arrow/projections → arrow`. The workaround is a local `arrowService` interface stub, `SetService()`, and `SetSyncInstall()` — all fragile mutation after construction.

---

## Design Decisions

1. **Each module folder exports one public interface.** The parent (orchestrator) calls the interface only; it never knows the submodule's internals.

2. **Each module has its own interface** for testability and mocking.

3. **Each module has a `projections.go` file** (convention enforced). Subscriptions are registered inside the module's constructor. If a module has no projections, the file is minimal but present.

4. **Errors stay at the use-case root** (`arrow/types.go`, `quiver/types.go`). Modules do not define their own sentinel errors.

5. **Simple constructors for submodules.** `New(...)` returns the interface directly. The builder pattern is reserved for the top-level use-case (arrow, quiver) and engines.

6. **One test file per source file.** Struct-only files (types, errors) are exempt.

7. **`commands/` and `upcasters/` are unchanged** and move into `internal/` as-is.

8. **Modules know about asynx.** Unlike engines, use-case modules may hold and use `asynx.Asynx` directly.

---

## Arrow — New Structure

```
internal/app/arrow/
├── arrow.go              ← ArrowService interface + arrowService struct (pure delegation)
├── arrow_test.go
├── builder.go            ← Builder pattern; wires all internal modules
├── builder_test.go
├── types.go              ← DTOs (ArrowListDTO, ArrowDetailDTO), sentinel errors (no test)
└── internal/
    ├── catalog/
    │   ├── catalog.go        ← Catalog interface + implementation
    │   ├── catalog_test.go
    │   ├── projections.go    ← OnArrowAdded, OnArrowUpdated, OnArrowRemoved (registered in New)
    │   ├── projections_test.go
    │   └── store/
    │       ├── store.go      ← ArrowCatalog interface + SQLite implementation (unchanged logic)
    │       └── store_test.go
    ├── execution/
    │   ├── execution.go      ← Execution interface + executionService struct (delegates to runner + installer)
    │   ├── execution_test.go
    │   ├── runner/
    │   │   ├── runner.go         ← Runner interface + implementation
    │   │   ├── runner_test.go
    │   │   ├── projections.go    ← WizardExecutor (runtime.begun), StopCoordinator (runtime.mark_stopping)
    │   │   ├── projections_test.go
    │   │   ├── stepreporter.go   ← StepReporter (used only by WizardExecutor)
    │   │   └── stepreporter_test.go
    │   └── installer/
    │       ├── installer.go      ← Installer interface + implementation
    │       ├── installer_test.go
    │       ├── projections.go    ← (convention file; no subscriptions yet)
    │       ├── projections_test.go
    │       └── deps/
    │           ├── handler.go    ← DependenciesHandler interface + implementation (no SetSyncInstall)
    │           └── handler_test.go
    ├── commands/             ← Unchanged (move from arrow/commands/ to arrow/internal/commands/)
    │   └── *.go + *_test.go
    └── upcasters/            ← Unchanged (move from arrow/upcasters/ to arrow/internal/upcasters/)
        └── upcasters.go
```

---

## Arrow — Module Interfaces

### `catalog.Catalog`

```go
type Catalog interface {
    Add(ctx context.Context, ns domain.Namespace) error
    Update(ctx context.Context, ns domain.Namespace) error
    Remove(ctx context.Context, ns domain.Namespace) error
    List(ctx context.Context) ([]domain.Arrow, error)
    Get(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
    HasDependents(ctx context.Context, ns domain.Namespace, excludeNs domain.Namespace) (bool, error)
}
```

`GetDetail` is implemented in `arrow.go` (the orchestrator). It assembles the DTO by calling `catalog.Get`, reading runtime state from `asynxRuntime` directly, and reading indirect deps from `vault` directly. This keeps catalog free of vault as a dependency. The orchestrator holds `asynxRuntime` and `vault` as extra fields solely for this method.

### `runner.Runner`

```go
type Runner interface {
    BeginExecution(ctx context.Context, ns domain.Namespace, method string, userVars map[string]string) error
    ExecuteSync(ctx context.Context, ns domain.Namespace, method string, userVars map[string]string) error
    Stop(ctx context.Context, ns domain.Namespace) error
}
```

Runner does not expose `GetWorkDir`. WizardExecutor (inside runner) calls vault directly — no external interface needed.

### `installer.Installer`

```go
type Installer interface {
    Install(ctx context.Context, ns domain.Namespace, userVars map[string]string) error
    Uninstall(ctx context.Context, ns domain.Namespace, userVars map[string]string) error
    CleanupAfterUninstall(ctx context.Context, ns domain.Namespace) error
}
```

### `execution.Execution`

Aggregates runner + installer into one interface so the orchestrator has a single handle:

```go
type Execution interface {
    BeginExecution(ctx context.Context, ns domain.Namespace, method string, userVars map[string]string) error
    Stop(ctx context.Context, ns domain.Namespace) error
    Install(ctx context.Context, ns domain.Namespace, userVars map[string]string) error
    Uninstall(ctx context.Context, ns domain.Namespace, userVars map[string]string) error
}
```

`ExecuteSync` is internal to execution/; the orchestrator never calls it directly.

---

## Arrow — Post-Execution Coordination (circular dep solution)

`WizardExecutor` (in `runner/`) needs to trigger two post-execution actions:

- After `_execute` is cancelled → run `_stop` (via `runner.BeginExecution`)
- After `_uninstall` succeeds → run `CleanupAfterUninstall` (via `installer.Installer`)

Since runner owns WizardExecutor but must not import installer (that would create runner ↔ installer circular deps), `execution.go` passes a `PostExecutionFn` callback into the runner at construction time:

```go
// runner/runner.go
type PostExecutionFn func(ctx context.Context, ns domain.Namespace, method string, execErr error, outcome ExecutionOutcome)

// Injected by execution/ after both runner and installer are constructed.
func (r *runnerService) SetPostExecutionHook(fn PostExecutionFn)
```

`execution.New()` constructs runner, constructs installer, then wires:

```go
run.SetPostExecutionHook(func(ctx, ns, method, execErr, outcome) {
    switch method {
    case "_execute":
        if errors.Is(execErr, context.Canceled) {
            _ = run.BeginExecution(ctx, ns, "_stop", nil)
        }
    case "_uninstall":
        if outcome == ExecutionOutcomeSuccess {
            _ = inst.CleanupAfterUninstall(ctx, ns)
        }
    }
})
```

No `SetService`. No local interface stub. No two-phase init on the public API.

---

## Arrow — `deps` Handler (no more `SetSyncInstall`)

`deps.New()` receives the runner interface directly:

```go
func New(
    depTree deptree.DepTree,
    vault vault.Vault,
    manifold manifold.Manifold,
    axArrow asynx.Asynx[domain.Arrow],
    axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
    runner runner.Runner,
) DependenciesHandler
```

`SetSyncInstall` is deleted. The handler calls `runner.ExecuteSync` directly. `DependenciesHandler` interface drops `SetSyncInstall`.

---

## Arrow — Construction Order (builder.go)

```
1. Open event stores (arrowES, runtimeES)
2. newAsynxArrow(arrowES)   → axArrow
3. newAsynxRuntime(runtimeES) → axRuntime
4. catalog.New(axArrow, axRuntime, engines.Vault, engines.Manifold) → cat
   └── internally: registers arrow.added / arrow.updated / arrow.removed projections
5. execution.New(axArrow, axRuntime, engines, os, cat) → exc
   └── internally:
       a. runner.New(axArrow, axRuntime, engines.Vault, engines.Netbridge, engines.Wizard, os) → run
          └── registers runtime.begun (WizardExecutor), runtime.mark_stopping (StopCoordinator)
       b. deps.New(engines.DepTree, engines.Vault, engines.Manifold, axArrow, axRuntime, run) → dep
          └── engines.Wizard.RegisterDispatch(StepTypeDependencies, wizard.Adapt(dep))
       c. installer.New(axArrow, axRuntime, engines.Vault, engines.DepTree, cat, run) → inst
       d. run.SetPostExecutionHook(wired to run + inst)
       e. returns executionService{runner: run, installer: inst}
6. return &arrowService{catalog: cat, execution: exc}
```

The arrow builder only imports `catalog` and `execution`. It never imports `runner`, `installer`, or `deps`.

---

## Arrow — Orchestrator (arrow.go)

`arrowService` holds two modules and delegates every method call:

```go
type arrowService struct {
    catalog   catalog.Catalog
    execution execution.Execution
    // asynxRuntime held for GetDetail (reads runtime state for DTO assembly)
    asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
    // vault held for GetDetail (reads indirect deps)
    vault vault.Vault
}
```

Every public method is a one-liner delegation. No logic lives here.

`GetDetail` is the one exception — it assembles `ArrowDetailDTO` by calling `catalog.Get`, then reading runtime state from `asynxRuntime` and indirect deps from `vault`. All three deps are held directly on `arrowService`.

---

## Arrow — Dependency Graph

```
catalog     →  (axArrow, axRuntime, vault, manifold only — no module deps)
runner      →  (axArrow, axRuntime, vault, netbridge, wizard, os — no module deps)
deps        →  runner
installer   →  catalog, runner
execution   →  catalog, runner, installer, deps (constructs and wires all four)
arrowService→  catalog, execution
```

No cycles. Two foundational modules (catalog, runner). One composed module (installer). One coordinator (execution).

---

## Quiver — New Structure

Quiver has no execution or runtime side. Its refactor is simpler: one module (`catalog`) plus the orchestrator.

```
internal/app/quiver/
├── quiver.go             ← QuiverService interface + quiverService struct (pure delegation)
├── quiver_test.go
├── builder.go            ← Builder pattern
├── builder_test.go
├── types.go              ← DTOs (QuiverListDTO, QuiverDetailDTO), sentinel errors (no test)
└── internal/
    ├── catalog/
    │   ├── catalog.go        ← Catalog interface + implementation (Add, Update, Remove, List, GetDetail)
    │   ├── catalog_test.go
    │   ├── projections.go    ← OnQuiverAdded, OnQuiverUpdated, OnQuiverRemoved
    │   ├── projections_test.go
    │   └── store/
    │       ├── store.go      ← QuiverCatalog interface + SQLite implementation
    │       └── store_test.go
    ├── commands/             ← Unchanged (move to internal/)
    │   └── *.go + *_test.go
    └── upcasters/            ← Unchanged (move to internal/)
        └── upcasters.go
```

### `catalog.Catalog` (quiver)

```go
type Catalog interface {
    Add(ctx context.Context, ns domain.Namespace) error
    Update(ctx context.Context, ns domain.Namespace) error
    Remove(ctx context.Context, ns domain.Namespace) error
    List(ctx context.Context) ([]domain.Quiver, error)
    GetDetail(ctx context.Context, ns domain.Namespace) (*domain.Quiver, error)
}
```

### Construction Order (quiver builder.go)

```
1. Open event store (quiverES)
2. newAsynxQuiver(quiverES) → axQuiver
3. catalog.New(axQuiver, engines.Vault, engines.Manifold) → cat
   └── registers quiver.added / quiver.updated / quiver.removed projections
4. return &quiverService{catalog: cat}
```

---

## What Does NOT Change

- **`app/container.go`** — still calls `arrow.NewArrowBuilder().Build()` and `quiver.NewQuiverBuilder().Build()`. No changes needed.
- **`commands/`** — logic unchanged; files physically move to `internal/commands/`.
- **`upcasters/`** — logic unchanged; files physically move to `internal/upcasters/`.
- **Domain types, engine interfaces** — untouched.
- **Import paths for `commands/` and `upcasters/`** within arrow must be updated to `arrow/internal/commands/` and `arrow/internal/upcasters/`.

---

## Test Convention

- One `_test.go` file per `.go` source file.
- Struct-only files (types, DTOs, sentinel errors, plain data models) are exempt.
- Existing tests at `arrow/arrow_test.go`, `arrow/execution_test.go`, `arrow/lifecycle_test.go` move into their owning module and are renamed to match the new source files.
- `arrow/integration_test.go` stays at the `arrow/` level — it tests the full wired service end-to-end.
- `arrow/coverage_test.go` stays at the `arrow/` level.

---

## Migration Map (old → new)

| Old path | New path |
|---|---|
| `arrow/arrow.go` | `arrow/arrow.go` (slimmed to delegation only) |
| `arrow/execution.go` | `arrow/internal/execution/runner/runner.go` |
| `arrow/lifecycle.go` | `arrow/internal/execution/installer/installer.go` |
| `arrow/builder.go` | `arrow/builder.go` (rewritten to wire internal modules) |
| `arrow/types.go` | `arrow/types.go` (unchanged) |
| `arrow/projections/arrow_events.go` | `arrow/internal/catalog/projections.go` |
| `arrow/projections/wizard_executor.go` | `arrow/internal/execution/runner/projections.go` |
| `arrow/projections/stop_coordinator.go` | `arrow/internal/execution/runner/projections.go` |
| `arrow/projections/container.go` | deleted (each module registers its own) |
| `arrow/projections/service.go` | deleted (circular dep workaround no longer needed) |
| `arrow/stepreporter/step_reporter.go` | `arrow/internal/execution/runner/stepreporter.go` |
| `arrow/store/store.go` | `arrow/internal/catalog/store/store.go` |
| `arrow/deps/handler.go` | `arrow/internal/execution/installer/deps/handler.go` |
| `arrow/commands/` | `arrow/internal/commands/` |
| `arrow/upcasters/` | `arrow/internal/upcasters/` |
| `quiver/projections/quiver_events.go` | `quiver/internal/catalog/projections.go` |
| `quiver/projections/container.go` | deleted |
| `quiver/store/store.go` | `quiver/internal/catalog/store/store.go` |
| `quiver/commands/` | `quiver/internal/commands/` |
| `quiver/upcasters/` | `quiver/internal/upcasters/` |
