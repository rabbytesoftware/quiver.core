# Quiver — Command Catalog

## Overview

Commands are the only way state changes in this system. Each command is a pure Go struct implementing the Asynx `Command[T]` interface — no I/O, no side effects, no cross-aggregate reads. The app layer performs all real work (git fetch, process launch, port assignment, variable resolution) before calling `asynx.Send()`. Commands only validate current state and record the transition.

> **Note:** Code snippets in this document are pseudocode and do not reflect the final implementation.

```go
type Command[T any] interface {
    AggregateID()   string
    Validate(current *T) error
    EmitEvent(current *T) T
    EventName()     string
    ShouldSnapshot() bool
}
```

### Naming Schema

Commands and events use dot notation: `aggregate.Action`. This enables precise regex subscriptions on the Asynx bus:

```go
asynx.Subscribe("^arrow\\.",         handleArrowEvent)     // all Arrow events
asynx.Subscribe("^runtime\\.",       handleRuntimeEvent)   // all Runtime events
asynx.Subscribe("^quiver\\.",        handleQuiverEvent)    // all Quiver events
asynx.Subscribe("runtime\\.Advance", handleStepFeed)       // step progress only
```

---

## Arrow Commands

`Arrow` is a stateless catalog entry — it holds namespace + manifest + removed flag. Lifecycle state belongs to `ArrowRuntime`. `Arrow` has no knowledge of execution.

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `arrow.Add` | `current == nil`, namespace valid, manifest name not empty | `arrow.Add` | yes |
| `arrow.UpdateManifest` | `current != nil && !current.Removed`, manifest name not empty | `arrow.UpdateManifest` | yes |
| `arrow.Remove` | `current != nil && !current.Removed` | `arrow.Remove` | yes |

---

### `arrow.Add`

App layer git-fetched the Arrow repository and parsed the manifest. This command stores it as a catalog entry. No `ArrowRuntime` exists yet — the Arrow has not been installed.

```go
type AddArrow struct {
    ArrowNamespace Namespace
    ArrowManifest  ArrowManifest
}

func (c AddArrow) AggregateID() string   { return c.ArrowNamespace.String() }
func (c AddArrow) EventName() string     { return "arrow.Add" }
func (c AddArrow) ShouldSnapshot() bool  { return true }

func (c AddArrow) Validate(current *Arrow) error {
    if current != nil {
        return ErrValidation("arrow already exists")
    }
    if err := c.ArrowNamespace.Validate(); err != nil {
        return err
    }
    if c.ArrowManifest.Name == "" {
        return ErrValidation("manifest name required")
    }
    return nil
}

func (c AddArrow) EmitEvent(current *Arrow) Arrow {
    return Arrow{
        Namespace: c.ArrowNamespace,
        Manifest:  c.ArrowManifest,
    }
}
```

---

### `arrow.UpdateManifest`

A new version was pulled from upstream. App layer fetches and parses the updated manifest, then sends this command. This is a catalog update, not a reinstall.

```go
type UpdateArrowManifest struct {
    ArrowNamespace Namespace
    ArrowManifest  ArrowManifest
}

func (c UpdateArrowManifest) AggregateID() string   { return c.ArrowNamespace.String() }
func (c UpdateArrowManifest) EventName() string     { return "arrow.UpdateManifest" }
func (c UpdateArrowManifest) ShouldSnapshot() bool  { return true }

func (c UpdateArrowManifest) Validate(current *Arrow) error {
    if current == nil {
        return ErrValidation("arrow does not exist")
    }
    if current.Removed {
        return ErrValidation("arrow has been removed")
    }
    if c.ArrowManifest.Name == "" {
        return ErrValidation("manifest name required")
    }
    return nil
}

// App layer is responsible for verifying ArrowRuntime.State == ready before sending.

func (c UpdateArrowManifest) EmitEvent(current *Arrow) Arrow {
    next := *current
    next.Manifest = c.ArrowManifest
    return next
}
```

---

### `arrow.Remove`

Tombstones the Arrow. Sets `Removed = true`. App layer checks this field and stops serving the catalog entry when set.

App layer is responsible for verifying `ArrowRuntime` is nil (never installed), `ArrowRuntime.State == removed` (uninstalled), or `ArrowRuntime.State == absent` (install failed) before sending. This is the only cross-aggregate check in the system and it lives at the app layer, not in `Validate()`.

```go
type RemoveArrow struct {
    ArrowNamespace Namespace
}

func (c RemoveArrow) AggregateID() string   { return c.ArrowNamespace.String() }
func (c RemoveArrow) EventName() string     { return "arrow.Remove" }
func (c RemoveArrow) ShouldSnapshot() bool  { return true }

func (c RemoveArrow) Validate(current *Arrow) error {
    if current == nil {
        return ErrValidation("arrow does not exist")
    }
    if current.Removed {
        return ErrValidation("arrow already removed")
    }
    return nil
}

func (c RemoveArrow) EmitEvent(current *Arrow) Arrow {
    next := *current
    next.Removed = true
    return next
}
```

---

## ArrowRuntime Commands

`ArrowRuntime` holds the execution context. It is initialized **lazily** — there is no explicit init command. `runtime.Begin` handles `current == nil` (first execution ever on this Arrow) by constructing the aggregate from scratch.

All execution passes through `ArrowRuntime` — lifecycle methods and user-defined methods alike. The `Execution.Method` field is the discriminator.

**Reserved lifecycle methods:**

- `"_install"` — install lifecycle steps
- `"_uninstall"` — uninstall lifecycle steps
- `"_execute"` — start a long-running process (optional)
- `"_stop"` — stop lifecycle steps, runs after `_execute` is cancelled (optional)

`_install`/`_uninstall` are always implicit — every Arrow goes through the install flow (Step 0 dependency resolution + any manifest-defined steps) even if the manifest omits them. `_execute`/`_stop` is an optional pair: if an Arrow defines one, it must define the other. Both pairs may be present.

Any other string is a user-defined method name.

### State preconditions (app layer responsibility)

`BeginExecution.Validate()` intentionally does **not** check state preconditions per method — it only verifies that no execution is in progress and the runtime exists (except for `_install`). The **use case layer** must enforce the following state preconditions before sending any command. These are cross-aggregate or state-specific checks that do not belong in the command's `Validate()`.

| Method | Required State | Additional Preconditions |
|--------|---------------|--------------------------|
| `_install` | `nil` (never installed) or `absent` (retry) | Arrow must exist in catalog (`Asynx[Arrow].Get` returns non-nil). No active ArrowRuntime execution. |
| `_execute` | `ready` | Arrow manifest must define `execute` lifecycle steps. |
| `_stop` | `running` | Sends `MarkStopping` (not `BeginExecution`). The use case layer dispatches `_stop` after `_execute` ends. |
| `_uninstall` | `ready` | — |
| Custom method | Must match `method.AvailableIn` | Method name must exist in `Arrow.Manifest.Methods`. Current state must be in the method's `available_in` list. |

For `arrow.Remove`, the app layer verifies that `ArrowRuntime` is nil (never installed), `state == removed` (uninstalled), or `state == absent` (install failed) before sending. This is documented on the `arrow.Remove` command.

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `runtime.Begin` | `Execution == nil` (nil current allowed for `_install`; `absent` state allowed for `_install` re-attempt); sets `State` based on method | `runtime.Begin` | yes |
| `runtime.MarkStopping` | `State == running` | `runtime.MarkStopping` | no |
| `runtime.Advance` | `Execution != nil`, index in bounds, transition valid | `runtime.Advance` | no |
| `runtime.End` | `Execution != nil`; sets `State` based on method + outcome, builds `LastReturn` | `runtime.End` | yes |

---

### `runtime.Begin`

App layer resolved variables, built the step list, and determined which method to run. This command creates the runtime if it has never existed (`current == nil`), or reuses the existing one if it does. The aggregate is stateless on variables — the use case layer always provides the full resolved set. The `Steps` field expects steps pre-initialized with `Status: StepStatusPending`.

**Install flow note:** For `_install`, `runtime.Begin` is sent **before** DepTree runs. The step list passed to `BeginExecution` includes a synthetic **Step 0** of type `dependencies` (title: "Resolving dependencies"), followed by the manifest's install steps re-indexed starting at 1:

```go
// App layer constructs steps for _install:
depStep := StepProgress{Index: 0, Status: StepStatusPending, Step: NewDependenciesStep("Resolving dependencies")}
installSteps := toStepProgress(manifest.Lifecycle.Install, startIndex: 1) // re-indexed from 1
allSteps := append([]StepProgress{depStep}, installSteps...)
```

After `runtime.Begin`, the app layer advances Step 0 to `running` and calls DepTree to resolve the full dependency graph. If DepTree fails (cycle, fetch error), Step 0 is advanced to `failed` with the error message via `runtime.Advance`, then `runtime.End{_install, failed}` transitions the arrow to `absent`. The failure reason is preserved in `LastReturn.Steps[0].Error`. If DepTree succeeds, Step 0 is advanced to `completed` and the app layer sends `arrow.Add` + `runtime.Begin{_install}` for each dependency in topological order, before executing the root arrow's install steps (starting from index 1 — the Wizard skips Step 0). After all installations complete, the Vault entry is updated with `indirect_dependencies` (see `vault.md` §4.5). See `deptree.md` §Call Site for the full flow.

```go
type BeginExecution struct {
    ArrowNamespace Namespace
    Method         string
    Variables      map[string]string
    Steps          []StepProgress
}

func (c BeginExecution) AggregateID() string   { return c.ArrowNamespace.String() }
func (c BeginExecution) EventName() string     { return "runtime.Begin" }
func (c BeginExecution) ShouldSnapshot() bool  { return true }

func (c BeginExecution) Validate(current *ArrowRuntime) error {
    // nil current is allowed only for _install — first execution initializes the runtime lazily
    // absent state is allowed for _install — retry after failed/cancelled install
    if current == nil && c.Method != "_install" {
        return ErrValidation("arrow has not been installed")
    }
    if current != nil && current.State == ArrowStateAbsent && c.Method != "_install" {
        return ErrValidation("arrow has not been installed")
    }
    if current != nil && current.Execution != nil {
        return ErrValidation("execution already in progress")
    }
    if c.Method == "" {
        return ErrValidation("method name required")
    }
    return nil
}

func (c BeginExecution) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := ArrowRuntime{
        Namespace: c.ArrowNamespace,
        Execution: &Execution{
            Method:    c.Method,
            Steps:     c.Steps,
            Variables: c.Variables,
        },
    }
    if current != nil {
        next.State = current.State
        next.LastReturn = current.LastReturn
    }
    switch c.Method {
    case "_install":
        next.State = ArrowStateInstalling
    case "_execute":
        next.State = ArrowStateRunning
    case "_stop":
        next.State = ArrowStateStopping
    case "_uninstall":
        next.State = ArrowStateUninstalling
    default:
        // Custom methods — state preserved from current
    }
    return next
}
```

---

### `runtime.Advance`

One step changed status. Fires twice per step:
1. `pending → running` when the step starts
2. `running → completed` or `running → failed` when it finishes

This is the real-time progress feed. WebSocket subscribers and projections that display execution progress listen to `runtime.Advance`.

```go
type AdvanceStep struct {
    ArrowNamespace Namespace
    StepIndex      int
    ToStatus       StepStatus
    Error          *string // set when ToStatus == StepStatusFailed
}

func (c AdvanceStep) AggregateID() string   { return c.ArrowNamespace.String() }
func (c AdvanceStep) EventName() string     { return "runtime.Advance" }
func (c AdvanceStep) ShouldSnapshot() bool  { return false }

func (c AdvanceStep) Validate(current *ArrowRuntime) error {
    if current == nil || current.Execution == nil {
        return ErrValidation("no execution in progress")
    }
    if c.StepIndex < 0 || c.StepIndex >= len(current.Execution.Steps) {
        return ErrValidation("step index out of bounds")
    }
    return nil
}

func (c AdvanceStep) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    exec := *current.Execution
    steps := make([]StepProgress, len(exec.Steps))
    copy(steps, exec.Steps)
    steps[c.StepIndex].Status = c.ToStatus
    steps[c.StepIndex].Error = c.Error
    exec.Steps = steps
    next.Execution = &exec
    return next
}
```

---

### `runtime.End`

Execution finished. Builds `LastReturn` from the current execution's method, steps, and variables plus the outcome provided by the use case layer. Clears `Execution` to nil. State transitions branch on method + outcome.

The use case layer maps Wizard results to outcomes:
- `err == nil` → `ExecutionOutcomeSuccess`
- `err == context.Canceled` → `ExecutionOutcomeCancelled`
- any other error → `ExecutionOutcomeFailed`

```go
type EndExecution struct {
    ArrowNamespace Namespace
    Outcome        ExecutionOutcome
}

func (c EndExecution) AggregateID() string   { return c.ArrowNamespace.String() }
func (c EndExecution) EventName() string     { return "runtime.End" }
func (c EndExecution) ShouldSnapshot() bool  { return true }

func (c EndExecution) Validate(current *ArrowRuntime) error {
    if current == nil || current.Execution == nil {
        return ErrValidation("no execution in progress")
    }
    return nil
}

func (c EndExecution) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    next.LastReturn = &Return{
        Method:    current.Execution.Method,
        Outcome:   c.Outcome,
        Steps:     current.Execution.Steps,
        Variables: current.Execution.Variables,
    }
    next.Execution = nil

    method := current.Execution.Method
    switch {
    case method == "_install" && c.Outcome == ExecutionOutcomeSuccess:
        next.State = ArrowStateReady
    case method == "_install": // failed or cancelled
        next.State = ArrowStateAbsent
    case method == "_execute":
        next.State = ArrowStateReady // success, failed, or cancelled — still installed
    case method == "_stop":
        next.State = ArrowStateReady // best-effort
    case method == "_uninstall" && c.Outcome == ExecutionOutcomeSuccess:
        next.State = ArrowStateRemoved
    case method == "_uninstall": // failed
        next.State = ArrowStateReady // rollback — still installed
    default:
        // Custom methods — state preserved
    }
    return next
}
```

**Uninstall cleanup flow:** When `_uninstall` completes with `success`, the use case layer performs **orphaned dependency cleanup**. For each dependency (direct + indirect, from Vault entry), the use case layer checks whether any other installed arrow references it. If no other arrow depends on it, the use case layer issues `runtime.Begin{_uninstall}` → Wizard executes uninstall steps → `runtime.End{_uninstall}` for the orphaned dependency. This cascades in **reverse topological order** (leaves first). Every command flows through Asynx — subscribers see each dependency being uninstalled as regular `runtime.*` events. See `deptree.md` §Uninstall Flow for the full sequence.

---

### `runtime.MarkStopping`

The user has requested that a running Arrow be stopped. Records the intent on the aggregate — the `StopCoordinator` subscription picks this up and signals the Wizard to cancel the ongoing `_execute` goroutine.

```go
type MarkStopping struct {
    ArrowNamespace Namespace
}

func (c MarkStopping) AggregateID() string  { return c.ArrowNamespace.String() }
func (c MarkStopping) EventName() string    { return "runtime.MarkStopping" }
func (c MarkStopping) ShouldSnapshot() bool { return false }

func (c MarkStopping) Validate(current *ArrowRuntime) error {
    if current == nil || current.State != ArrowStateRunning {
        return ErrValidation("arrow must be running")
    }
    return nil
}

func (c MarkStopping) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    next.State = ArrowStateStopping
    return next
}
```

---

## Quiver Commands

`Quiver` is the catalog definition — a list of Arrows published together. Since Asynx is append-only, `Quiver` carries a `Removed bool` field as a tombstone. App layer checks this field and stops serving the catalog when set.

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `quiver.Add` | `current == nil`, namespace valid, manifest name not empty | `quiver.Add` | yes |
| `quiver.UpdateManifest` | `current != nil && !current.Removed` | `quiver.UpdateManifest` | yes |
| `quiver.Remove` | `current != nil && !current.Removed` | `quiver.Remove` | yes |

---

### `quiver.Add`

App layer git-fetched the Quiver repository and parsed the manifest. Stores it.

```go
type AddQuiver struct {
    QuiverNamespace Namespace
    QuiverManifest  QuiverManifest
}

func (c AddQuiver) AggregateID() string   { return c.QuiverNamespace.String() }
func (c AddQuiver) EventName() string     { return "quiver.Add" }
func (c AddQuiver) ShouldSnapshot() bool  { return true }

func (c AddQuiver) Validate(current *Quiver) error {
    if current != nil {
        return ErrValidation("quiver already exists")
    }
    if err := c.QuiverNamespace.Validate(); err != nil {
        return err
    }
    if c.QuiverManifest.Name == "" {
        return ErrValidation("manifest name required")
    }
    return nil
}

func (c AddQuiver) EmitEvent(current *Quiver) Quiver {
    return Quiver{
        Namespace: c.QuiverNamespace,
        Manifest:  c.QuiverManifest,
    }
}
```

---

### `quiver.UpdateManifest`

New version pulled from upstream. Manifest replaced in place.

```go
func (c UpdateQuiverManifest) AggregateID() string   { return c.QuiverNamespace.String() }
func (c UpdateQuiverManifest) EventName() string     { return "quiver.UpdateManifest" }
func (c UpdateQuiverManifest) ShouldSnapshot() bool  { return true }

func (c UpdateQuiverManifest) Validate(current *Quiver) error {
    if current == nil || current.Removed {
        return ErrValidation("quiver does not exist or has been removed")
    }
    return nil
}
```

---

### `quiver.Remove`

Tombstones the Quiver. Sets `Removed = true`. App layer checks this field and stops serving the catalog.

```go
func (c RemoveQuiver) AggregateID() string   { return c.QuiverNamespace.String() }
func (c RemoveQuiver) EventName() string     { return "quiver.Remove" }
func (c RemoveQuiver) ShouldSnapshot() bool  { return true }

func (c RemoveQuiver) Validate(current *Quiver) error {
    if current == nil || current.Removed {
        return ErrValidation("quiver does not exist or already removed")
    }
    return nil
}

func (c RemoveQuiver) EmitEvent(current *Quiver) Quiver {
    next := *current
    next.Removed = true
    return next
}
```
