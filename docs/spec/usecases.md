# Quiver — Use Case Layer

## Overview

The use case layer is the **orchestration layer**. It is the only layer that touches multiple infrastructure modules. It owns all Asynx instances, composes Vault+Manifold, drives the install flow through DepTree, coordinates execution lifecycle, and resolves variables. No business logic lives in infrastructure modules — they are tools the use case layer calls.

Two modules:

- **Arrow** (`internal/app/arrow/`) — catalog CRUD, execution lifecycle, install/uninstall flows, variable resolution
- **Quiver** (`internal/app/quiver/`) — catalog CRUD only

Related specs: [commands.md](commands.md), [domain.md](domain.md), [wizard.md](wizard.md), [vault.md](vault.md), [manifold.md](manifold.md), [deptree.md](deptree.md), [netbridge.md](netbridge.md), [subscriptions.md](subscriptions.md), [http-api.md](http-api.md).

---

## 1. Module Structure

### 1.1 Arrow Module — `internal/app/arrow/`

| File | Responsibility |
|------|---------------|
| `service.go` | `ArrowService` struct, constructor, dependency injection |
| `catalog.go` | Add, Remove, Update, List, GetDetail — catalog CRUD on `Asynx[Arrow]`, cross-aggregate checks |
| `runtime.go` | `beginExecution`, `executeSync`, stop flow coordination, variable resolution (6-layer merge), `asynxStepReporter`, `handleExecutionError` |
| `installer.go` | Install flow: Step 0 management, DepTree → per-dep install loop, Vault `indirect_dependencies` update, rollback on failure |
| `uninstaller.go` | Uninstall flow: reverse dependency check, root uninstall, orphan detection, reverse-topo orphan cleanup |
| `resolver.go` | `resolveManifest` (Vault cache-first + Manifold fallback), DepTree resolver callback construction |
| `asynx.go` | Asynx instance setup for `Asynx[Arrow]` + `Asynx[ArrowRuntime]` |

#### Sub-packages — one file per item:

**`commands/`** — one file per command struct (struct + `AggregateID()`, `Validate()`, `EmitEvent()`, `EventName()`, `ShouldSnapshot()`):

| File | Command |
|------|---------|
| `add.go` | `AddArrow` |
| `update_manifest.go` | `UpdateArrowManifest` |
| `remove.go` | `RemoveArrow` |
| `begin_execution.go` | `BeginExecution` |
| `advance_step.go` | `AdvanceStep` |
| `end_execution.go` | `EndExecution` |
| `mark_stopping.go` | `MarkStopping` |

**`projections/`** — one file per subscription handler:

| File | Handler |
|------|---------|
| `stop_coordinator.go` | Calls `wizard.Cancel(namespace)` on `runtime.MarkStopping` event |
| `websocket_runtime.go` | WebSocketHub registration for `^runtime\.` events |
| `websocket_arrow.go` | WebSocketHub registration for `^arrow\.` events |

**`upcasters/`** — one file per upcaster (event schema migration).

### 1.2 Quiver Module — `internal/app/quiver/`

| File | Responsibility |
|------|---------------|
| `service.go` | `QuiverService` struct, constructor |
| `catalog.go` | Add, Remove, Update, List, GetDetail on `Asynx[Quiver]` |
| `resolver.go` | `resolveManifest` for quivers (same cache-first pattern) |
| `asynx.go` | Asynx instance setup for `Asynx[Quiver]` |

**`commands/`** — one file per command struct:

| File | Command |
|------|---------|
| `add.go` | `AddQuiver` |
| `update_manifest.go` | `UpdateQuiverManifest` |
| `remove.go` | `RemoveQuiver` |

**`projections/`** — one file per handler:

| File | Handler |
|------|---------|
| `websocket_quiver.go` | WebSocketHub registration for `^quiver\.` events |

**`upcasters/`** — one file per upcaster.

---

## 2. Service Structs & Dependencies

### 2.1 ArrowService

```go
type ArrowService struct {
    asynxArrow   *asynx.Asynx[Arrow]
    asynxRuntime *asynx.Asynx[ArrowRuntime]
    vault        vault.Vault
    manifold     manifold.Manifold
    deptree      deptree.DepTree
    netbridge    netbridge.Netbridge
    wizard       *wizard.Wizard
    os           string // target OS for manifest resolution
}

func NewArrowService(
    asynxArrow   *asynx.Asynx[Arrow],
    asynxRuntime *asynx.Asynx[ArrowRuntime],
    vault        vault.Vault,
    manifold     manifold.Manifold,
    deptree      deptree.DepTree,
    netbridge    netbridge.Netbridge,
    wizard       *wizard.Wizard,
    os           string,
) *ArrowService
```

### 2.2 QuiverService

```go
type QuiverService struct {
    asynxQuiver *asynx.Asynx[Quiver]
    vault       vault.Vault
    manifold    manifold.Manifold
}

func NewQuiverService(
    asynxQuiver *asynx.Asynx[Quiver],
    vault       vault.Vault,
    manifold    manifold.Manifold,
) *QuiverService
```

### 2.3 Dependency Map Per File

| File | Infrastructure Dependencies |
|------|----------------------------|
| `catalog.go` | `asynxArrow`, `asynxRuntime` (for state in List/GetDetail), `vault` (for `indirect_dependencies` in GetDetail) |
| `runtime.go` | `asynxRuntime`, `wizard`, `netbridge` |
| `installer.go` | `asynxArrow`, `asynxRuntime` (via runtime.go), `deptree`, `vault` |
| `uninstaller.go` | `asynxRuntime` (via runtime.go), `vault`, `wizard` |
| `resolver.go` | `vault`, `manifold` |

All files are methods on `ArrowService` — they access dependencies through `svc.*`.

---

## 3. Arrow Catalog Operations (`catalog.go`)

### 3.1 Add

```go
func (svc *ArrowService) Add(ctx context.Context, ns Namespace) error
```

1. Validate namespace format
2. Call `svc.resolveManifest(ctx, ns)` — Vault cache-first, Manifold on miss
3. Send `AddArrow{ns, manifest}` to `asynxArrow`
4. Return error if validation fails (arrow already exists, invalid namespace)

**Errors:** `ErrInvalidNamespace`, `ErrAlreadyExists`, `ErrFetchFailed`

### 3.2 Update

```go
func (svc *ArrowService) Update(ctx context.Context, ns Namespace) error
```

1. Verify arrow exists and is not removed via `asynxArrow.Get(ns)`
2. Verify `ArrowRuntime` is nil or in `ready` state via `asynxRuntime.Get(ns)`
3. Force-fetch manifest from Manifold (bypass Vault cache)
4. Persist to Vault
5. Send `UpdateArrowManifest{ns, manifest}` to `asynxArrow`

**Errors:** `ErrNotFound`, `ErrRemoved`, `ErrStateViolation` (not ready), `ErrFetchFailed`

### 3.3 Remove

```go
func (svc *ArrowService) Remove(ctx context.Context, ns Namespace) error
```

1. Verify arrow exists via `asynxArrow.Get(ns)`
2. Cross-aggregate check: `asynxRuntime.Get(ns)` must be nil (never installed), `state == removed` (uninstalled), or `state == absent` (install failed)
3. Send `RemoveArrow{ns}` to `asynxArrow`

**Errors:** `ErrNotFound`, `ErrAlreadyRemoved`, `ErrStateViolation` (runtime still active)

### 3.4 List

```go
func (svc *ArrowService) List(ctx context.Context) ([]ArrowListDTO, error)
```

1. Get all arrows from `asynxArrow.GetAll()`, filter out `Removed == true`
2. For each arrow, query `asynxRuntime.Get(ns)` to get current state
3. Return list DTOs with namespace, name, version, description, state, tags, removed

### 3.5 GetDetail

```go
func (svc *ArrowService) GetDetail(ctx context.Context, ns Namespace) (*ArrowDetailDTO, error)
```

1. Get arrow from `asynxArrow.Get(ns)` — 404 if nil
2. Get runtime from `asynxRuntime.Get(ns)` — may be nil (never installed)
3. Query `vault.GetArrow(ctx, ns)` for `IndirectDependencies` — may be nil (pre-install)
4. Assemble full detail DTO: manifest fields + state + execution + last_return + indirect_dependencies + method names

**Errors:** `ErrNotFound`

---

## 4. Execution Lifecycle (`runtime.go`)

### 4.1 `beginExecution` — async execution

```go
func (svc *ArrowService) beginExecution(ctx context.Context, ns Namespace, method string, userVars map[string]string) error
```

Called for `_execute`, `_stop` (post-cancel dispatch), and custom methods. Returns immediately after launching a goroutine.

> **Note:** `installer.go` and `uninstaller.go` do NOT call `beginExecution`. The install flow manages `BeginExecution`/`EndExecution` commands and `wizard.Execute` calls directly (because of Step 0 management and the per-dependency loop). For per-dependency installs, `installer.go` calls `executeSync`. The uninstall flow similarly manages its own command sequence. See §5 and §6.

1. Get arrow from `asynxArrow.Get(ns)` — error if nil
2. Resolve variables via `svc.resolveVariables(ns, arrow.Manifest, method, userVars)`
3. Build step list from manifest for the given method
4. Get working directory from `vault.GetArrow(ctx, ns)`
5. Determine `AvailableIn` for the method:
   - `_install`: not set (nil/absent handling is separate in Validate)
   - `_execute`: `[ArrowStateReady]`
   - `_stop`: `[ArrowStateReady]` (the HTTP handler sends `MarkStopping`, not `BeginExecution`; but after `_execute` is cancelled and state returns to `ready`, the use case layer dispatches `BeginExecution{_stop}` via `handleExecutionError`)
   - `_uninstall`: `[ArrowStateReady]`
   - Custom: `arrow.Manifest.Methods[method].AvailableIn`
6. Send `BeginExecution{ns, method, availableIn, vars, steps}` to `asynxRuntime`
7. Build `wizard.ExecutionRequest` and `asynxStepReporter` (with index offset 1 for `_install`, 0 otherwise)
8. Launch goroutine:
   ```go
   go func() {
       err := svc.wizard.Execute(ctx, req, reporter)
       outcome := mapOutcome(err) // nil→success, context.Canceled→cancelled, other→failed
       svc.asynxRuntime.Send(EndExecution{ns, outcome})
       if err != nil {
           svc.handleExecutionError(ns, method, err)
       }
   }()
   ```
9. Return nil

### 4.2 `executeSync` — blocking execution

```go
func (svc *ArrowService) executeSync(ctx context.Context, ns Namespace, method string) error
```

Same as `beginExecution` but **blocking** — does not launch a goroutine. Used by `installer.go` for per-dependency installs. The installer's own goroutine is the long-lived one; inside it, each dependency install runs synchronously.

1. Same steps 1–6 as `beginExecution`
2. Call `svc.wizard.Execute(ctx, req, reporter)` directly (blocking)
3. Send `EndExecution{ns, outcome}` to `asynxRuntime`
4. Return the error (or nil)

### 4.3 Stop Flow

```go
func (svc *ArrowService) Stop(ctx context.Context, ns Namespace) error
```

1. Verify `ArrowRuntime.State == running` via `asynxRuntime.Get(ns)`
2. Send `MarkStopping{ns}` to `asynxRuntime`
3. Return nil — the rest happens reactively

**Reactive flow (via StopCoordinator subscription in `projections/stop_coordinator.go`):**

```
runtime.MarkStopping event fires
  → StopCoordinator calls wizard.Cancel(namespace)
  → _execute goroutine's context is cancelled
  → wizard.Execute returns with context.Canceled
  → goroutine sends EndExecution{_execute, cancelled}
  → runtime.State = ready (transient)
```

**Post-cancel dispatch (inside handleExecutionError):**

When `_execute` ends with `cancelled` outcome and the arrow has stop lifecycle steps:

```go
func (svc *ArrowService) handleExecutionError(ns Namespace, method string, err error) {
    if method == "_execute" && errors.Is(err, context.Canceled) {
        arrow, _ := svc.asynxArrow.Get(ns.String())
        if arrow.Manifest.Lifecycle.Stop != nil {
            svc.beginExecution(ctx, ns, "_stop", nil)
        }
    }
}
```

The full stop sequence is documented in [wizard.md § Stop Flow](wizard.md#stop-flow--full-sequence).

### 4.4 Variable Resolution

```go
func (svc *ArrowService) resolveVariables(ns Namespace, manifest ArrowManifest, method string, userVars map[string]string) map[string]string
```

Assembles variables in 6 layers. Later layers override earlier ones:

| Priority | Source | Implementation |
|----------|--------|----------------|
| 1 (lowest) | Built-in variables | `INSTALL_PATH` from Vault home path, `ARROW_NAMESPACE` from ns, `PLATFORM` from `svc.os` |
| 2 | Dependency built-in variables | For each dep in `manifest.Dependencies`: query `vault.GetArrow(dep)` for home path, prefix with dep namespace (e.g., `github.com/valve/steamcmd.INSTALL_PATH`) |
| 3 | Manifest variable defaults | `manifest.Variables[].Default` as string |
| 4 | Netbridge port allocations | Call `netbridge.Allocate(ctx, ns.String(), protocol, preferred)` for each `manifest.Netbridge` entry. Map port name → allocated port as string. On failure: if `required` → abort, if not → skip. |
| 5 | Stored variables | `asynxRuntime.Get(ns).LastReturn.Variables` (if a previous execution exists) |
| 6 (highest) | User-provided overrides | `userVars` from HTTP request body |

After merging, walk all step commands and replace `${VAR}` tokens with resolved values.

**Port deallocation:** Ports are deallocated after the **final execution** for a namespace completes — after `_stop` ends (if the stop flow was triggered), or after `_execute` ends naturally (no stop). For `_install`, `_uninstall`, and custom methods, ports are deallocated when that execution ends. The use case layer calls `netbridge.DeallocateByOwner(ctx, ns.String())` to release all allocated ports. This ensures stop lifecycle steps can reference allocated ports (e.g., for sending signals to processes bound to specific ports).

### 4.5 `asynxStepReporter`

Implements `wizard.StepReporter`. Translates Wizard callbacks into Asynx `AdvanceStep` commands with an index offset.

```go
type asynxStepReporter struct {
    namespace   Namespace
    asynx       *asynx.Asynx[ArrowRuntime]
    indexOffset int // 1 for _install (Step 0 managed externally), 0 for everything else
}

func (r *asynxStepReporter) OnStepStarted(index int) {
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index + r.indexOffset,
        ToStatus:       StepStatusRunning,
    })
}

func (r *asynxStepReporter) OnStepCompleted(index int) {
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index + r.indexOffset,
        ToStatus:       StepStatusCompleted,
    })
}

func (r *asynxStepReporter) OnStepFailed(index int, err error) {
    errStr := err.Error()
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index + r.indexOffset,
        ToStatus:       StepStatusFailed,
        Error:          &errStr,
    })
}
```

---

## 5. Install Flow (`installer.go`)

### 5.1 Entry Point

```go
func (svc *ArrowService) Install(ctx context.Context, ns Namespace, userVars map[string]string) error
```

1. Validate arrow exists via `asynxArrow.Get(ns)` — 404 if nil
2. Validate runtime is nil or `absent` via `asynxRuntime.Get(ns)` — 422 if in another state
3. Launch goroutine: `go svc.runInstall(ctx, ns, userVars)`
4. Return nil (HTTP handler returns 202)

### 5.2 `runInstall` — the goroutine

```go
func (svc *ArrowService) runInstall(ctx context.Context, ns Namespace, userVars map[string]string)
```

**Phase 1 — Begin and resolve dependencies:**

1. Get arrow manifest
2. Resolve variables (6-layer merge)
3. Build step list: `[depStep(index 0), ...installSteps(index 1+)]`
4. Send `BeginExecution{ns, "_install", availableIn, vars, steps}` → state: `installing`
5. Advance Step 0 to `running`
6. Build resolver callback: `func(ctx, depNs) → manifest.Dependencies` (using `resolveManifest`)
7. Call `deptree.Resolve(ctx, ns, resolver)` → `orderedDeps []Namespace`
8. If DepTree fails:
   - Advance Step 0 to `failed` with error
   - Send `EndExecution{ns, failed}` → state: `absent`
   - Return
9. Advance Step 0 to `completed`

**Phase 2 — Install dependencies in topological order:**

```go
var installed []Namespace // track for rollback

for _, dep := range orderedDeps {
    if dep == ns {
        continue // skip root — installed last
    }

    // Skip if already installed (runtime exists and state != absent)
    rt, _ := svc.asynxRuntime.Get(dep.String())
    if rt != nil && rt.State != ArrowStateAbsent {
        continue
    }

    // Resolve manifest and add to catalog if not present
    manifest, _, err := svc.resolveManifest(ctx, dep)
    if err != nil {
        svc.rollbackInstalled(ctx, installed)
        svc.sendEndExecution(ns, ExecutionOutcomeFailed)
        return
    }

    existing, _ := svc.asynxArrow.Get(dep.String())
    if existing == nil {
        svc.asynxArrow.Send(AddArrow{dep, *manifest})
    }

    // Install dependency synchronously
    err = svc.executeSync(ctx, dep, "_install")
    if err != nil {
        svc.rollbackInstalled(ctx, installed)
        svc.sendEndExecution(ns, ExecutionOutcomeFailed)
        return
    }

    installed = append(installed, dep)
}
```

**Phase 3 — Install root arrow:**

1. Build `wizard.ExecutionRequest` with manifest install steps (index 1+, skip Step 0)
2. Build `asynxStepReporter` with `indexOffset: 1`
3. Call `wizard.Execute(ctx, req, reporter)` — blocking
4. Send `EndExecution{ns, outcome}`
5. If success: update Vault with `indirect_dependencies`

### 5.3 Rollback

```go
func (svc *ArrowService) rollbackInstalled(ctx context.Context, installed []Namespace)
```

Uninstalls already-installed dependencies in **reverse order**. Best-effort — log failures, continue with remaining.

```go
for i := len(installed) - 1; i >= 0; i-- {
    dep := installed[i]
    rt, _ := svc.asynxRuntime.Get(dep.String())
    if rt == nil || rt.State != ArrowStateReady {
        continue // not in ready state — skip
    }
    err := svc.executeSync(ctx, dep, "_uninstall")
    if err != nil {
        // Log the error, continue rollback
        log.Warn("rollback: failed to uninstall %s: %v", dep, err)
    }
}
```

### 5.4 Vault Update After Install

```go
func (svc *ArrowService) updateIndirectDeps(ctx context.Context, ns Namespace, deptreeResult []Namespace) error
```

After DepTree returns `[]Namespace` in topological order and install succeeds, compute indirect dependencies (all transitive deps not in `manifest.Dependencies`) and update Vault:

```go
directSet := toSet(manifest.Dependencies)
var indirect []Namespace
for _, dep := range deptreeResult {
    if dep == ns { continue }
    if !directSet[dep.String()] {
        indirect = append(indirect, dep)
    }
}
svc.vault.PutArrow(ctx, ns, manifest, indirect)
```

### 5.5 Concurrent Install Conflict

If two arrows are being installed concurrently and share a dependency, the second install will hit `BeginExecution.Validate()` → `"execution already in progress"` when attempting to install the shared dependency. The second install **fails entirely** — the user retries later when the shared dependency has finished installing.

---

## 6. Uninstall Flow (`uninstaller.go`)

### 6.1 Entry Point

```go
func (svc *ArrowService) Uninstall(ctx context.Context, ns Namespace, userVars map[string]string) error
```

1. Validate arrow exists and runtime is in `ready` state
2. **Reverse dependency check**: scan all Vault entries — if any other installed arrow lists `ns` in its `dependencies` or `indirect_dependencies`, reject with 422 (`"other arrows depend on this arrow"`)
3. Launch goroutine: `go svc.runUninstall(ctx, ns, userVars)`
4. Return nil (HTTP handler returns 202)

### 6.2 Reverse Dependency Check

```go
func (svc *ArrowService) hasDependents(ctx context.Context, ns Namespace) (bool, error)
```

Iterates all arrows using `asynxArrow.GetAll()`, filters to installed arrows via `asynxRuntime.Get()`, then checks Vault for dependency data:

1. Get all arrows from `asynxArrow.GetAll()`
2. For each arrow (skip the target namespace itself, skip removed arrows):
   a. Check `asynxRuntime.Get(ns)` — skip if nil or state is `absent`/`removed`
   b. Get Vault entry: `vault.GetArrow(ctx, ns)`
   c. Check if target `ns` appears in `entry.Manifest.Dependencies` or `entry.IndirectDependencies`
   d. If found in any → return true
3. If no arrow references the target → return false

This is O(arrows × deps) — acceptable for the expected scale (tens of arrows, not thousands).

### 6.3 `runUninstall` — the goroutine

```go
func (svc *ArrowService) runUninstall(ctx context.Context, ns Namespace, userVars map[string]string)
```

**Phase 1 — Uninstall root arrow:**

1. Resolve variables, build uninstall step list
2. Send `BeginExecution{ns, "_uninstall", [ArrowStateReady], vars, steps}` → state: `uninstalling`
3. Call `wizard.Execute(ctx, req, reporter)` — blocking
4. Send `EndExecution{ns, outcome}`
5. If failed → state: `ready` (rollback per state machine). Return.

**Phase 2 — Orphaned dependency cleanup (on success only):**

1. Get root's Vault entry: `dependencies` + `indirect_dependencies`
2. For each dep in the combined list:
   - Check if any OTHER installed arrow references this dep (same scan as `hasDependents`, excluding the root)
   - If no other arrow references it → dep is orphaned
3. Determine reverse topological order by calling `deptree.Resolve(ctx, ns, vaultResolver)` where `vaultResolver` reads dependency lists from Vault (all manifests are cached — zero network cost). Reverse the result to get leaves-first order. Filter to only orphaned namespaces.
4. For each orphaned dep:
   - `svc.executeSync(ctx, dep, "_uninstall")` — blocking
   - If uninstall fails → dep transitions to `ready` (rollback). Log failure, continue with remaining orphans.
5. Clean up Vault entries for successfully removed arrows: `vault.DeleteArrow(ctx, dep)`

**Phase 3 — Clean up root Vault entry:**

```go
svc.vault.DeleteArrow(ctx, ns)
```

---

## 7. Manifest Resolution (`resolver.go`)

### 7.1 `resolveManifest` — cache-first

```go
func (svc *ArrowService) resolveManifest(ctx context.Context, ns Namespace) (*ArrowManifest, string, error)
```

Uses `svc.os` internally when calling `manifold.ResolveArrow(ctx, ns, svc.os)`. The `os` parameter is not exposed in the signature — it comes from the service's configured target OS.

1. Check Vault: `vault.GetArrow(ctx, ns)`
2. **Fresh hit** (`err == nil`): return `entry.Manifest, homePath, nil`
3. **Stale** (`err == ErrStale`): try Manifold refresh
   - If Manifold fails → graceful degradation: return stale entry with logged warning
   - If Manifold succeeds → `vault.PutArrow(ctx, ns, manifest, nil)`, return fresh manifest
4. **Not cached** (`err == ErrNotCached`): full fetch from Manifold
   - If Manifold fails → return error
   - If Manifold succeeds → `vault.PutArrow(ctx, ns, manifest, nil)`, return manifest

### 7.2 DepTree Resolver Callback

```go
func (svc *ArrowService) buildDepResolver() deptree.ResolverFunc {
    return func(ctx context.Context, ns Namespace) ([]Namespace, error) {
        manifest, _, err := svc.resolveManifest(ctx, ns)
        if err != nil {
            return nil, err
        }
        return manifest.Dependencies, nil
    }
}
```

### 7.3 Quiver Resolver

`QuiverService` has an analogous `resolveManifest` using `vault.GetQuiver` / `manifold.ResolveQuiver` / `vault.PutQuiver`.

---

## 8. Quiver Catalog Operations (`quiver/catalog.go`)

Quiver is purely catalog CRUD — no execution lifecycle, no runtime, no dependencies.

### 8.1 Add

```go
func (svc *QuiverService) Add(ctx context.Context, ns Namespace) error
```

1. Validate namespace format
2. Resolve manifest: `svc.resolveManifest(ctx, ns)`
3. Send `AddQuiver{ns, manifest}`

### 8.2 Update

```go
func (svc *QuiverService) Update(ctx context.Context, ns Namespace) error
```

1. Verify quiver exists and is not removed
2. Force-fetch from Manifold (bypass cache)
3. Persist to Vault
4. Send `UpdateQuiverManifest{ns, manifest}`

### 8.3 Remove

```go
func (svc *QuiverService) Remove(ctx context.Context, ns Namespace) error
```

1. Verify quiver exists
2. Send `RemoveQuiver{ns}`
3. Call `vault.DeleteQuiver(ctx, ns)`

### 8.4 List

```go
func (svc *QuiverService) List(ctx context.Context) ([]QuiverListDTO, error)
```

### 8.5 GetDetail

```go
func (svc *QuiverService) GetDetail(ctx context.Context, ns Namespace) (*QuiverDetailDTO, error)
```

---

## 9. Error Handling Strategy

### 9.1 Error Types

The use case layer defines its own error types for the HTTP layer to map to status codes:

```go
var (
    ErrNotFound        = errors.New("not found")
    ErrAlreadyExists   = errors.New("already exists")
    ErrAlreadyRemoved  = errors.New("already removed")
    ErrStateViolation  = errors.New("state violation")
    ErrFetchFailed     = errors.New("fetch failed")
    ErrInvalidNamespace = errors.New("invalid namespace")
    ErrDependentsExist = errors.New("other arrows depend on this arrow")
)
```

### 9.2 Wizard Error Mapping

The use case layer maps Wizard execution results to `ExecutionOutcome`:

| Wizard result | Outcome | State transition |
|--------------|---------|------------------|
| `err == nil` | `success` | Method-dependent (see `commands.md` § `runtime.End`) |
| `err == context.Canceled` | `cancelled` | Method-dependent |
| Any other error | `failed` | Method-dependent |

### 9.3 HTTP Error Mapping

The HTTP layer (not the use case layer) maps use case errors to status codes:

| Use case error | HTTP Status |
|----------------|-------------|
| `ErrInvalidNamespace` | `400 Bad Request` |
| `ErrNotFound` | `404 Not Found` |
| `ErrAlreadyExists` | `409 Conflict` |
| `ErrAlreadyRemoved` | `409 Conflict` |
| `ErrStateViolation` | `422 Unprocessable Entity` |
| `ErrDependentsExist` | `422 Unprocessable Entity` |
| `ErrFetchFailed` | `502 Bad Gateway` |

---

## 10. Subscriptions & Projections

### 10.1 StopCoordinator (`projections/stop_coordinator.go`)

Registered on `Asynx[ArrowRuntime]` with pattern `runtime\.MarkStopping`.

```go
func StopCoordinator(wizard *wizard.Wizard) asynx.ProjectionHandler[ArrowRuntime] {
    return func(event asynx.Event[ArrowRuntime]) error {
        wizard.Cancel(event.Aggregate.Namespace)
        return nil
    }
}
```

### 10.2 WebSocket Projections

The WebSocketHub lives in the **API layer** (`internal/api/`). The projection files in `projections/` register Asynx subscription handlers that push DTOs to the hub.

**Arrow module registers:**
- `^runtime\.` → push ArrowRuntime DTO to `/v1/arrow.runtime` channels
- `^arrow\.` → push Arrow DTO to `/v1/arrow` channels

**Quiver module registers:**
- `^quiver\.` → push Quiver DTO to `/v1/quiver` channels

See [subscriptions.md](subscriptions.md) for the full subscription table and [websocket.md](websocket.md) for DTO shapes.

---

## 11. Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Interfaces defined in infrastructure packages** | Each module (`vault`, `manifold`, `deptree`, `netbridge`) defines its own interface. The app layer imports and depends on them. No "Port" suffix. |
| **`AvailableIn` on `BeginExecution`** | State gating for all methods (lifecycle + custom) is validated inside `BeginExecution.Validate()`. The app layer populates `AvailableIn` from the manifest. |
| **Install rollback** | If a dependency install fails mid-chain, already-installed dependencies are uninstalled in reverse order. Best-effort — log failures, continue rolling back. |
| **Reverse dependency check on uninstall** | Before `_uninstall` begins, the use case layer checks if any other installed arrow depends on the target. If yes, reject with 422. |
| **Concurrent install conflict** | If a shared dependency is already being installed, the second install fails entirely. `BeginExecution.Validate()` naturally rejects with "execution already in progress". |
| **`GetDetail` queries Vault** | `indirect_dependencies` is sourced from the Vault entry, not the domain aggregate. `GetDetail` queries both `Asynx[Arrow]` and Vault. |
| **WebSocketHub in API layer** | The hub is a delivery mechanism for real-time client updates — an API concern, not infrastructure. |
| **Singular package names** | `arrow/`, `quiver/` — follows Go convention. |
| **One file per command/projection/upcaster** | Each command, projection handler, and upcaster lives in its own file within its sub-package. |

---

## 12. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should `resolveManifest` in `resolver.go` be shared between `ArrowService` and `QuiverService` via a common helper, or duplicated per module? | Duplicated — the two resolvers call different Vault/Manifold methods (`GetArrow`/`ResolveArrow` vs `GetQuiver`/`ResolveQuiver`). The pattern is identical but the types differ. |
| 2 | Should the use case layer validate `requirements` (CPU, RAM, disk, OS) before allowing install? | Not in v0 — requirements are informational for the frontend. |
| 3 | Should `_stop` dispatch (after `_execute` cancellation) be synchronous or async? | Async via `beginExecution` — the stop flow is a separate execution with its own goroutine. |
