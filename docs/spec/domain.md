# Quiver — Domain Model

## Overview

The domain layer is built around three aggregates: `Arrow`, `ArrowRuntime`, and `Quiver`. Each uses `Namespace` as its aggregate ID — no UUID needed, namespaces are globally unique by design.

Asynx (the event sourcing library) stores RFC 6902 JSON diffs between state transitions, not full snapshots. Fat aggregates are fine — only changed fields are stored per event.

---

## 1. `Arrow` Aggregate

The durable catalog entry. Built when an Arrow is added — the manifest is git-fetched, parsed, and stored. Purely catalog — no state, no lifecycle. Runtime state lives on `ArrowRuntime`.

```go
type Arrow struct {
    Namespace Namespace
    Manifest  ArrowManifest
    Removed   bool
}
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

`Install`/`Uninstall` are always implicit — the platform guarantees the install flow runs (including Step 0 dependency resolution) even if these slices are `nil`/empty. `Execute`/`Stop` is an optional pair — `nil` Execute means this is a package Arrow (install-and-done, no long-running process). If one side of execute/stop is defined, the other must be too.

```go
type Lifecycle struct {
    Install   []Step  // paired with Uninstall
    Execute   []Step  // paired with Stop — empty = package Arrow (no long-running process)
    Stop      []Step  // paired with Execute
    Uninstall []Step  // paired with Install
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

```go
type Step interface {
    Type() StepType
    Title() string
    ExitOnFailure() bool
}
```

### `BasicStep`

Embedded into every concrete step type. Holds the common fields and satisfies the `Step` interface methods. All fields are unexported — set at construction via YAML decoding. Accessed through interface methods.

```go
type BasicStep struct {
    stepType      StepType // unexported
    exitOnFailure bool     // unexported — default true at construction
    title         string   // unexported
}

func (bs BasicStep) Type() StepType          { return bs.stepType }
func (bs BasicStep) Title() string           { return bs.title }
func (bs BasicStep) ExitOnFailure() bool     { return bs.exitOnFailure }
```

### Concrete step types

```go
type RunStep struct {
    BasicStep
    Command string
    Timeout time.Duration
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

type DependenciesStep struct {
    BasicStep
}
```

### Constructors

The Assembler (manifold module) constructs steps from parsed YAML. Since `BasicStep` fields are unexported, constructors are required for cross-package creation.

```go
func NewRunStep(title string, command string, timeout time.Duration, exitOnFailure bool) RunStep
func NewFetchStep(title string, url string, to string, timeout time.Duration, exitOnFailure bool) FetchStep
func NewSignalStep(title string, signal string, timeout time.Duration, exitOnFailure bool) SignalStep
func NewDependenciesStep(title string) DependenciesStep
```

`DependenciesStep` is a synthetic step type — never authored in manifests. The app layer injects it as **Step 0** of every `_install` execution to represent the DepTree dependency resolution phase. `ExitOnFailure` is always `true` (if resolution fails, the install cannot proceed).

**The Wizard never receives this step.** The app layer manages Step 0 progress directly via `runtime.Advance`, then passes only the manifest's install steps (index 1+) to the Wizard. The use case layer's `StepReporter` implementation applies an **index offset of 1** when translating Wizard callbacks to `runtime.Advance` commands — the Wizard's step 0 maps to runtime index 1, step 1 maps to index 2, etc. See `deptree.md` §Call Site for the full install flow and `wizard.md` §StepReporter for the offset pattern.

### `StepType`

```go
type StepType string

const (
    StepTypeRun          StepType = "run"
    StepTypeFetch        StepType = "fetch"
    StepTypeSignal       StepType = "signal"
    StepTypeDependencies StepType = "dependencies"
)
```

Step construction from raw YAML data is handled by the Manifold module's Assembler concern — see `manifold.md` §9.

---

## 3. `ArrowRuntime` Aggregate

The volatile execution context. Created lazily when `_install` begins. Tells the complete story: what's happening now (`Execution`) and what happened last (`LastReturn`).

### `ArrowState`

`ArrowRuntime` owns the lifecycle state. `nil` ArrowRuntime means the Arrow has never been installed. `absent` means install was attempted but failed or was cancelled.

```
nil ──[BeginExecution{_install}]──→ installing ──[EndExecution{success}]──→ ready
  ↑                                     │                                    ↑    ↓ [BeginExecution{_execute}]
  │                                     └── [EndExecution{failed|cancelled}] │  running
  │                                         → absent                         │    ↓ [MarkStopping]
  │                                             │                            │  stopping ←─[BeginExecution{_stop}]─┐
  │                                             │ [BeginExecution{_install}]  │    │                                │
  │                                             └── → installing (retry)     │    ├── [EndExecution{_stop}] ───→ ready
  │                                                                          │    │
  │                                                                          └────┴── [EndExecution{_execute}] (natural exit — no stop needed)
  │
ready ──[BeginExecution{_uninstall}]──→ uninstalling ──[EndExecution{success}]──→ removed
                                                    └── [EndExecution{failed}]──→ ready (rollback)
```

**State transitions branch on execution outcome.** The full transition table is in `commands.md` under `runtime.End`.

**Stop flow detail:** When a running Arrow is stopped, the full state sequence is:
`running` → `stopping` (MarkStopping) → EndExecution{_execute, cancelled} → `ready` → BeginExecution{_stop} → `stopping` → EndExecution{_stop} → `ready`.
The brief `ready` between the two executions is a transient state — the use case layer dispatches `_stop` immediately after `_execute` ends.

```go
type ArrowState string

const (
    ArrowStateAbsent       ArrowState = "absent"       // runtime exists, install failed or cancelled
    ArrowStateInstalling   ArrowState = "installing"
    ArrowStateReady        ArrowState = "ready"
    ArrowStateRunning      ArrowState = "running"
    ArrowStateStopping     ArrowState = "stopping"
    ArrowStateUninstalling ArrowState = "uninstalling"
    ArrowStateRemoved      ArrowState = "removed"
)
```

```go
type ArrowRuntime struct {
    Namespace  Namespace
    State      ArrowState
    Execution  *Execution  // nil when idle
    LastReturn *Return     // nil if no execution has ever completed
}
```

### `Execution`

```go
type Execution struct {
    Method    string
    Steps     []StepProgress
    Variables map[string]string  // resolved variables (includes port assignments)
}
```

> **No process ID on the domain.** The Wizard tracks processes internally by namespace (via Runtime's deterministic UUID v5 key). The domain aggregate does not need to know which OS process is running — that is an infrastructure concern managed entirely within the Wizard's `processKeys` map. See `runtime.md` §Process Key.

### `Return`

Records the outcome of the most recent completed execution. A complete forensic record — outcome, final step statuses, and variables used.

```go
type Return struct {
    Method    string
    Outcome   ExecutionOutcome
    Steps     []StepProgress
    Variables map[string]string
}
```

### `ExecutionOutcome`

```go
type ExecutionOutcome string

const (
    ExecutionOutcomeSuccess   ExecutionOutcome = "success"
    ExecutionOutcomeFailed    ExecutionOutcome = "failed"
    ExecutionOutcomeCancelled ExecutionOutcome = "cancelled"
)
```

### `StepProgress`

Tracks the execution progress of each step. Uses the same `Step` types as the manifest — no separate record type. When stored in `ArrowRuntime`, step fields hold resolved values (variables already expanded). Steps are initialized with `StepStatusPending` when constructed for `BeginExecution`.

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
| `ArrowState` | `domain/arrow_state.go` | New. Belongs to `ArrowRuntime`. Enum: `absent`, `installing`, `ready`, `running`, `stopping`, `uninstalling`, `removed`. |
| `ExecutionOutcome` | `domain/execution.go` | New. Enum: `success`, `failed`, `cancelled`. |
| `StepType` | `domain/step.go` | New. Enum: `run`, `fetch`, `signal`, `dependencies`. |
| `StepStatus` | `domain/step.go` | New. Enum: `pending`, `running`, `completed`, `failed`. |
| `PortDef` | `domain/port.go` | Replaces `PortRule` — simpler name for Arrow manifest port definitions. |

---

## 6. Aggregate Coordination

`Arrow` and `ArrowRuntime` share the same `Namespace` as aggregate ID but live in separate Asynx instances (`Asynx[Arrow]` and `Asynx[ArrowRuntime]`).

The **app layer** coordinates between them — the aggregates have no knowledge of each other.

- `Arrow` is purely catalog — namespace + manifest, no state
- `ArrowRuntime` owns the state machine — it transitions state on every lifecycle command. Created lazily on first `_install` (`BeginExecution` handles `current == nil`)
- Failure model: state transitions branch on execution outcome. A failed `_install` transitions to `absent` (not installed). A failed `_execute` transitions to `ready` (still installed). `LastReturn` records the full forensic record of what happened.
