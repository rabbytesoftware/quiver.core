# Dependencies Handler Refactor

**Date:** 2026-04-05
**Branch:** feature/app-layer

## Problem

`_install` is the only method with special-cased execution logic. It diverges from every other method in three ways:

1. `runInstall()` exists as a separate code path — `Install()` calls `go runInstall()` while every other method goes through `BeginExecution()` → `beginExecution()` → wizard.
2. `DependenciesStep` is injected into `BeginExecution.Steps` for aggregate tracking, but manually advanced by `runInstall()` directly — the wizard never sees it.
3. `beginExecution()` carries an `indexOffset` (`1` for `_install`, `0` for everything else) to compensate for the dep step sitting at index 0 in the aggregate while the wizard runs user steps starting at wizard-index 0.

Additionally, `executeSync()` is called with `"_install"` for transitive dep installs, which goes through `beginExecution("_install")` — this path passes `DependenciesStep` to the wizard (which returns `ErrUnknownStepType` and marks it failed+continues due to `ExitOnFailure() == false`), and the `indexOffset=1` produces wrong step indices for user steps in the aggregate.

## Solution

Register a `DependenciesHandler` in the wizard's dispatch map. The wizard runs `DependenciesStep` like any other step — at its natural index, with full reporter coverage. All special casing in the app layer disappears.

```
wizard.Execute([depStep, userStep1, userStep2])
  i=0  depStep   → DependenciesHandler.Execute() → dep resolution
  i=1  userStep1 → RunHandler.Execute()
  i=2  userStep2 → RunHandler.Execute()
```

Wizard index == aggregate index. No offset anywhere.

## Components

### New: `internal/app/arrow/deps/handler.go`

`DependenciesHandler` implements `wizstep.Handler[domainstep.DependenciesStep]`.

Its `Execute()` contains all logic currently in `runInstall()` after the `BeginExecution` send:
- Advance step 0 → running
- Resolve dep tree via `DepTree.Resolve`
- For each uninstalled dep: resolve manifest, add to arrow aggregate if missing, call `syncInstall`
- Advance step 0 → completed/failed
- Rollback on failure (reverse-order uninstall of installed deps)
- On success: `updateIndirectDeps`

Dependencies injected at construction:
```go
type DependenciesHandler struct {
    depTree      deptree.DepTree
    vault        vault.Vault
    manifold     manifold.Manifold
    asynxArrow   asynx.Asynx[domain.Arrow]
    asynxRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
    catalog      ArrowCatalog
    syncInstall  func(ctx, ns, method, vars) error  // breaks circular dep
}
```

The `syncInstall` func is a closure over `arrowService.executeSync`, set after service construction (two-phase init in builder).

### New: `wizard.Option` + `wizard.WithHandler[S]`

`wizard.New()` gains a functional options parameter:

```go
type Option func(*wizard)

func WithHandler[S domainstep.Step](t domainstep.StepType, h wizstep.Handler[S]) Option {
    return func(w *wizard) {
        adapt(w.dispatch, t, h)
    }
}

func New(opts ...Option) (Wizard, error) { ... }
```

External packages call `wizard.WithHandler(domainstep.StepTypeDependencies, depsHandler)` and pass the result to `wizard.New()`. External packages cannot construct arbitrary `Option` values since `wizard` is unexported — only `WithHandler` is usable.

## Modified Components

### `arrow/builder.go`

Construction sequence to break the circular dep:

```
1. newAsynxArrow, newAsynxRuntime
2. depHandler := deps.NewHandler(depTree, vault, manifold, axArrow, axRuntime, catalog, nil)
3. wizard.New(wizard.WithHandler(StepTypeDependencies, depHandler))
4. svc := &arrowService{...}
5. depHandler.SetSyncInstall(svc.executeSync)
```

### `arrow/arrow.go`

`Install()` becomes:

```go
func (svc *arrowService) Install(ctx, ns, userVars) error {
    // state validation (unchanged)
    go func() {
        _ = svc.BeginExecution(ctx, ns, "_install", userVars)
    }()
    return nil
}
```

`runInstall()` is deleted.

### `arrow/execution.go`

- Remove `indexOffset` block from `beginExecution()`
- Remove `indexOffset` parameter from `stepreporter.New()` call
- `stepsForMethod("_install")` is unchanged — still returns `[depStep, ...installSteps]`

### `stepreporter/step_reporter.go`

- Remove `indexOffset int` field and constructor parameter
- All `i + r.indexOffset` → `i`

### `engine/container.go`

`wizard.New()` call gains the `DependenciesHandler` option — passed from the app container, not the engine container. The engine container keeps `wizard.New()` with no options; the app builder overrides/replaces the wizard with a fully wired one.

> **Note:** The current `engine.Container` creates the wizard. Since `DependenciesHandler` is app-layer, we have two options:
> A. App builder constructs its own wizard (ignoring the one in engine.Container) — clean but wastes engine.Container's wizard.
> B. Engine container exposes a way to register handlers post-construction.
> **Chosen: A.** App builder constructs the wizard directly with all handlers. Engine container's wizard is not used by the arrow service. Engine container still creates a bare wizard for use by other parts of the system if needed.

## Data Flow (after)

```
Install(ns, vars)
  → state validation
  → go BeginExecution(ns, "_install", vars)
       → beginExecution() → stepsForMethod returns [depStep, userSteps...]
       → BeginExecution event emitted → aggregate.ActiveRun = [depStep, userSteps...]
       → wizard.Execute([depStep, userSteps...], reporter{offset: 0})
            i=0 depStep   → DependenciesHandler.Execute(ns)
                              → AdvanceStep{0, running}
                              → DepTree.Resolve → install each dep via executeSync
                              → AdvanceStep{0, completed/failed}
            i=1 userStep1 → RunHandler → AdvanceStep{1, running/completed}
            i=2 userStep2 → RunHandler → AdvanceStep{2, running/completed}
       → EndExecution emitted
```

Recursive dep install via `executeSync(dep, "_install")`:
- Same path: wizard runs `[depStep, depUserSteps...]`
- `DependenciesHandler` runs at i=0, handles any transitive deps
- Clean, no special casing

## Open Decision: `DependenciesStep.ExitOnFailure()`

Currently `DependenciesStep.ExitOnFailure()` returns `false`. With the handler injected, if dep resolution fails the wizard would mark step 0 failed and **continue** to user steps — which is wrong.

`ExitOnFailure()` must return `true` for `DependenciesStep`. This is a one-line change in `dependencies.go` but is a domain model change and should be explicit.

## Open Decision: Wizard in Engine vs App Container

The engine container currently constructs the wizard. `DependenciesHandler` is app-layer and can't be in the engine container. Two options:

**A (chosen):** App builder constructs its own wizard with `WithHandler`. Engine container keeps a bare wizard for other consumers (or drops it entirely if no other consumers exist).

**B:** Engine container exposes `RegisterHandler` post-construction; app builder calls it during `Build()`.

Option A is cleaner — the wizard the arrow service uses is fully owned by the arrow builder. Verify no other part of the system uses `engine.Container.Wizard` after the app layer is built.

## Testing

- `DependenciesHandler` unit tests: mock `syncInstall`, verify dep resolution logic, rollback on partial failure, indirect dep update
- `wizard` unit test: `WithHandler` registers and dispatches correctly
- `arrow` integration test: `Install()` with deps goes through full path, step indices correct in aggregate
- `stepreporter` test: remove `indexOffset` tests, verify direct index passthrough
