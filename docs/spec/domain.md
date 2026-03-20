# Quiver — Domain Model

## Overview

The domain layer is built around three aggregates: `Arrow`, `ArrowRuntime`, and `Quiver`. Each uses `Namespace` as its aggregate ID — no UUID needed, namespaces are globally unique by design.

Asynx (the event sourcing library) stores RFC 6902 JSON diffs between state transitions, not full snapshots. Fat aggregates are fine — only changed fields are stored per event.

---

## 1. `Arrow` Aggregate

The durable catalog entry. Built when an Arrow is added — the manifest is git-fetched, parsed, and stored. Owns the lifecycle state machine.

```go
type Arrow struct {
    Namespace Namespace
    State     ArrowState
    Manifest  ArrowManifest
}
```

### `ArrowState`

```
absent → ready → running → stopped → removed
                  ↑          ↓
                  └──────────┘ (execute/stop loop)
```

```go
type ArrowState string

const (
    ArrowStateAbsent  ArrowState = "absent"
    ArrowStateReady   ArrowState = "ready"
    ArrowStateRunning ArrowState = "running"
    ArrowStateStopped ArrowState = "stopped"
    ArrowStateRemoved ArrowState = "removed"
)
```

### `ArrowManifest`

```go
type ArrowManifest struct {
    Name         string
    Description  string
    Version      string
    License      string
    URL          string
    Maintainers  []string
    Credits      []string
    Tags         []string
    Requirements Requirement
    Dependencies []Namespace   // full namespaces — validated via Namespace type
    Variables    []Variable
    Netbridge    []PortDef
    Lifecycle    Lifecycle
    Methods      map[string]Method
}
```

### `Lifecycle`

Pointer slices — `nil` is the meaningful zero value. A `nil` Execute means this is a package Arrow (install-and-done, no long-running process).

```go
type Lifecycle struct {
    Install   []Step
    Execute   []Step  // empty = package Arrow (no long-running process)
    Stop      []Step  // required when Execute is defined
    Uninstall []Step
}
```

### `Method`

Developer-defined actions gated by lifecycle state.

```go
type Method struct {
    AvailableIn []ArrowState
    Steps       []Step
}
```

---

## 2. `Step` Interface

Steps are the primitive execution unit. Shared between `Arrow` (manifest definition) and `ArrowRuntime` (execution progress). Three types: `run`, `fetch`, `signal`.

### `BasicStep`

Embedded into every concrete step type. Holds the common fields and satisfies the `Step` interface methods. `stepType` is unexported — only set at construction.

```go
type BasicStep struct {
    stepType StepType // unexported
    Title    string
}

func (bs BasicStep) Type()  StepType { return bs.stepType }
func (bs BasicStep) Title() string   { return bs.Title }
```

### Concrete step types

```go
type RunStep struct {
    BasicStep
    Command       string
    Timeout       time.Duration
    ExitOnFailure bool
}

type FetchStep struct {
    BasicStep
    URL     string
    To      string
    Timeout time.Duration
}

type SignalStep struct {
    BasicStep
    Signal  string
    Timeout time.Duration
}
```

### `StepType`

```go
type StepType string

const (
    StepTypeRun    StepType = "run"
    StepTypeFetch  StepType = "fetch"
    StepTypeSignal StepType = "signal"
)
```

### Parsing from YAML

`BasicStep` is the discriminator. Decode the node twice: first into `BasicStep` to get the type, then into the concrete struct for the full fields.

```go
func ParseStep(node *yaml.Node) (Step, error) {
    var base BasicStep
    node.Decode(&base)

    switch base.Type() {
    case StepTypeRun:
        var s RunStep
        node.Decode(&s)
        return s, nil
    case StepTypeFetch:
        var s FetchStep
        node.Decode(&s)
        return s, nil
    case StepTypeSignal:
        var s SignalStep
        node.Decode(&s)
        return s, nil
    default:
        return nil, ErrUnknownStepType
    }
}
```

The switch is intentional — adding a new step type requires the platform to know how to execute it anyway.

---

## 3. `ArrowRuntime` Aggregate

The volatile execution context. Created when a method begins executing. Holds resolved state — variables are expanded, `${VAR}` syntax is gone.

```go
type ArrowRuntime struct {
    Namespace        Namespace
    CurrentExecution *Execution        // nil when idle
    Variables        map[string]string // resolved variables (includes port assignments)
}
```

### `Execution`

```go
type Execution struct {
    Method string
    PID    *int  // nil unless a process is running (execute lifecycle)
    Steps  []StepProgress
}
```

### `StepProgress`

Tracks the execution progress of each step. Uses the same `Step` types as the manifest — no separate record type. When stored in `ArrowRuntime`, step fields hold resolved values (variables already expanded).

```go
type StepProgress struct {
    Index  int
    Status StepStatus
    Error  *string
    Step   Step // same type as manifest — resolved at execution time
}
```

### `StepStatus`

```go
type StepStatus string

const (
    StepStatusPending   StepStatus = "pending"
    StepStatusRunning   StepStatus = "running"
    StepStatusCompleted StepStatus = "completed"
    StepStatusFailed    StepStatus = "failed"
)
```

---

## 4. `Quiver` Aggregate

The catalog/store definition. Also git-fetched.

```go
type Quiver struct {
    Namespace Namespace
    Manifest  QuiverManifest
    Removed   bool
}

type QuiverManifest struct {
    Name        string
    Description string
    URL         string
    Maintainers []string
    Tags        []string
    Media       QuiverMedia
    Arrows      []Namespace // local AUIDs or full namespaces — validated via Namespace type
}

type QuiverMedia struct {
    Icon   string
    Banner string
}
```

---

## 5. Supporting Types

| Type | File | Notes |
|------|------|-------|
| `Namespace` | `domain/namespace.go` | Existing. Used as aggregate ID and for `Dependencies`, `Arrows`. |
| `Variable` | `domain/variable.go` | Existing. Keep as-is. |
| `Requirement` | `domain/requirement.go` | Existing. Keep as-is. |
| `Protocol` | `domain/protocol.go` | Existing. Keep as-is. |
| `ArrowState` | `domain/arrow_state.go` | New. Enum: `absent`, `ready`, `running`, `stopped`, `removed`. |
| `StepType` | `domain/step.go` | New. Enum: `run`, `fetch`, `signal`. |
| `StepStatus` | `domain/step.go` | New. Enum: `pending`, `running`, `completed`, `failed`. |
| `PortDef` | `domain/port.go` | Replaces `PortRule` — simpler name for Arrow manifest port definitions. |

---

## 6. Aggregate Coordination

`Arrow` and `ArrowRuntime` share the same `Namespace` as aggregate ID but live in separate Asynx instances (`Asynx[Arrow]` and `Asynx[ArrowRuntime]`).

The **app layer** coordinates between them — the aggregates have no knowledge of each other.

- `Arrow` owns the state machine — it transitions state on every lifecycle command
- `ArrowRuntime` is created lazily — only on first execution (`BeginExecution` handles `current == nil`)
- Failure model: risky work (process launch, port assignment) happens before state is committed. If it fails, `Arrow` stays in its current state.
