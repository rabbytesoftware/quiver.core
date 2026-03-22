# Quiver — Command Catalog

## Overview

Commands are the only way state changes in this system. Each command is a pure Go struct implementing the Asynx `Command[T]` interface — no I/O, no side effects, no cross-aggregate reads. The app layer performs all real work (git fetch, process launch, port assignment, variable resolution) before calling `asynx.Send()`. Commands only validate current state and record the transition.

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

`Arrow` is the durable catalog entry. It owns the manifest and the lifecycle state machine. It has no knowledge of execution — that belongs to `ArrowRuntime`.

```
absent → ready → removed
```

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `arrow.Add` | `current == nil`, namespace valid, manifest name not empty | `arrow.Add` | yes |
| `arrow.UpdateManifest` | `current != nil`, manifest name not empty | `arrow.UpdateManifest` | yes |
| `arrow.Remove` | `current != nil` | `arrow.Remove` | yes |

---

### `arrow.Add`

App layer git-fetched the Arrow repository and parsed the manifest. This command stores it. State is set to `absent` — install has not run yet.

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

A new version was pulled from upstream. App layer fetches and parses the updated manifest, then sends this command. State stays `ready` — this is not a reinstall.

```go
type UpdateArrowManifest struct {
    ArrowNamespace Namespace
    ArrowManifest  ArrowManifest
}

func (c UpdateArrowManifest) Validate(current *Arrow) error {
    if current == nil {
        return ErrValidation("arrow does not exist")
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

Removes the Arrow from the catalog. Since Asynx is append-only, the record is tombstoned by the absence of future events — app layer stops serving any Arrow whose catalog entry has been removed.

App layer is responsible for verifying `ArrowRuntime` is nil (never installed) or `ArrowRuntime.State == removed` (uninstalled) before sending. This is the only cross-aggregate check in the system and it lives at the app layer, not in `Validate()`.

```go
type RemoveArrow struct {
    ArrowNamespace Namespace
}

func (c RemoveArrow) Validate(current *Arrow) error {
    if current == nil {
        return ErrValidation("arrow does not exist")
    }
    return nil
}

func (c RemoveArrow) EmitEvent(current *Arrow) Arrow {
    return *current
}
```

---

## ArrowRuntime Commands

`ArrowRuntime` holds the execution context. It is initialized **lazily** — there is no explicit init command. `runtime.Begin` handles `current == nil` (first execution ever on this Arrow) by constructing the aggregate from scratch.

All execution passes through `ArrowRuntime` — install, uninstall, and user-defined methods alike. The `Execution.Method` field is the discriminator:

- `"_install"` — install lifecycle steps
- `"_uninstall"` — uninstall lifecycle steps
- any other string — user-defined method name

| Command | Validates | EventName | Snapshot |
|---|---|---|---|
| `runtime.Begin` | `CurrentExecution == nil` (nil current allowed — first use); sets `State = running` when method is `_execute` | `runtime.Begin` | yes |
| `runtime.MarkStopping` | `State == running` | `runtime.MarkStopping` | no |
| `runtime.RecordPID` | `CurrentExecution != nil`, PID not already set | `runtime.RecordPID` | no |
| `runtime.Advance` | `CurrentExecution != nil`, index in bounds, transition valid | `runtime.Advance` | no |
| `runtime.End` | `CurrentExecution != nil`; sets `State` based on method | `runtime.End` | yes |

---

### `runtime.Begin`

App layer resolved variables, built the step list, and determined which method to run. This command creates the runtime if it has never existed (`current == nil`), or reuses the existing one if it does. Incoming variables are merged with any previously stored ones — incoming values override.

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
    if c.Method == "_execute" {
        next.State = ArrowStateRunning
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
