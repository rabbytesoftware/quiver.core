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

App layer is responsible for verifying `ArrowRuntime` is nil (never installed) or `ArrowRuntime.State == removed` (uninstalled) before sending. This is the only cross-aggregate check in the system and it lives at the app layer, not in `Validate()`.

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

All four are optional, but they come in required pairs: if an Arrow defines `_install`, it must define `_uninstall`. If it defines `_execute`, it must define `_stop`. An Arrow must define at least one pair. Both pairs may be present.

Any other string is a user-defined method name.

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `runtime.Begin` | `CurrentExecution == nil` (nil current allowed — first use); sets `State = installing` for `_install`, `State = running` for `_execute`, `State = stopping` for `_stop` | `runtime.Begin` | yes |
| `runtime.MarkStopping` | `State == running` | `runtime.MarkStopping` | no |
| `runtime.RecordPID` | `CurrentExecution != nil`, PID not already set | `runtime.RecordPID` | no |
| `runtime.Advance` | `CurrentExecution != nil`, index in bounds, transition valid | `runtime.Advance` | no |
| `runtime.End` | `CurrentExecution != nil`; sets `State` based on method | `runtime.End` | yes |

---

### `runtime.Begin`

App layer resolved variables, built the step list, and determined which method to run. This command creates the runtime if it has never existed (`current == nil`), or reuses the existing one if it does. Incoming variables are merged with any previously stored ones — incoming values override. The `Steps` field expects steps pre-initialized with `Status: StepStatusPending`.

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
    // nil current is allowed — first execution initializes the runtime lazily
    if current != nil && current.CurrentExecution != nil {
        return ErrValidation("execution already in progress")
    }
    if c.Method == "" {
        return ErrValidation("method name required")
    }
    return nil
}

func (c BeginExecution) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    vars := c.Variables
    if current != nil && len(current.Variables) > 0 {
        merged := make(map[string]string, len(current.Variables)+len(vars))
        for k, v := range current.Variables {
            merged[k] = v
        }
        for k, v := range vars {
            merged[k] = v // incoming overrides
        }
        vars = merged
    }
    next := ArrowRuntime{
        Namespace: c.ArrowNamespace,
        Variables: vars,
        CurrentExecution: &Execution{
            Method: c.Method,
            Steps:  c.Steps,
        },
    }
    if current != nil {
        next.State = current.State
    }
    switch c.Method {
    case "_install":
        next.State = ArrowStateInstalling
    case "_execute":
        next.State = ArrowStateRunning
    case "_stop":
        next.State = ArrowStateStopping
    }
    return next
}
```

---

### `runtime.RecordPID`

A process was spawned by the app layer (relevant for `execute` lifecycle only). Stores the PID inside `CurrentExecution`. Separate from `runtime.Begin` so the process can start without blocking the execution record.

```go
type RecordPID struct {
    ArrowNamespace Namespace
    PID            int
}

func (c RecordPID) AggregateID() string   { return c.ArrowNamespace.String() }
func (c RecordPID) EventName() string     { return "runtime.RecordPID" }
func (c RecordPID) ShouldSnapshot() bool  { return false }

func (c RecordPID) Validate(current *ArrowRuntime) error {
    if current == nil || current.CurrentExecution == nil {
        return ErrValidation("no execution in progress")
    }
    if current.CurrentExecution.PID != nil {
        return ErrValidation("PID already recorded")
    }
    return nil
}

func (c RecordPID) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    exec := *current.CurrentExecution
    exec.PID = &c.PID
    next.CurrentExecution = &exec
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
    if current == nil || current.CurrentExecution == nil {
        return ErrValidation("no execution in progress")
    }
    if c.StepIndex < 0 || c.StepIndex >= len(current.CurrentExecution.Steps) {
        return ErrValidation("step index out of bounds")
    }
    return nil
}

func (c AdvanceStep) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    exec := *current.CurrentExecution
    steps := make([]StepProgress, len(exec.Steps))
    copy(steps, exec.Steps)
    steps[c.StepIndex].Status = c.ToStatus
    steps[c.StepIndex].Error = c.Error
    exec.Steps = steps
    next.CurrentExecution = &exec
    return next
}
```

---

### `runtime.End`

Execution finished — success or failure. Clears `CurrentExecution` to nil. The runtime goes idle. Variables are preserved so the next execution picks them up without re-resolving from scratch.

```go
type EndExecution struct {
    ArrowNamespace Namespace
}

func (c EndExecution) AggregateID() string   { return c.ArrowNamespace.String() }
func (c EndExecution) EventName() string     { return "runtime.End" }
func (c EndExecution) ShouldSnapshot() bool  { return true }

func (c EndExecution) Validate(current *ArrowRuntime) error {
    if current == nil || current.CurrentExecution == nil {
        return ErrValidation("no execution in progress")
    }
    return nil
}

func (c EndExecution) EmitEvent(current *ArrowRuntime) ArrowRuntime {
    next := *current
    next.CurrentExecution = nil
    switch current.CurrentExecution.Method {
    case "_install":
        next.State = ArrowStateReady
    case "_execute":
        next.State = ArrowStateReady // natural exit or after stop
    case "_stop":
        next.State = ArrowStateReady
    case "_uninstall":
        next.State = ArrowStateRemoved
    }
    return next
}
```

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
