# Quiver — Domain Model

## Overview

The domain layer is built around three aggregates: `Arrow`, `ArrowRuntime`, and `Quiver`.

- `Arrow` is keyed by `Namespace` — the namespace identifies a piece of software, not a version.
- `ArrowRuntime` is keyed by `ArrowRef` — a `(namespace, version)` pair, because each installed version has an independent lifecycle.
- `Quiver` is keyed by `Namespace`.

Asynx stores RFC 6902 JSON diffs between state transitions. Fat aggregates are fine — only changed fields are stored per event.

Cross-references: [versioning.md](arrow/v0/versioning.md) · [manifest.md](arrow/v0/manifest.md)

---

## 1. `ArrowRef`

The identity of a specific installed version of an Arrow. Used as the key for `ArrowRuntime`, `Vault` entries, and dependency declarations.

```go
type ArrowRef struct {
    Namespace Namespace
    Version   string // empty = "latest" (HEAD of default branch)
}

func (r ArrowRef) String() string // "github.com/valve/steamcmd@v1.2.3" or "github.com/valve/steamcmd" if latest
```

`ArrowRef` with an empty `Version` and `ArrowRef` with `Version = "latest"` are equivalent — both refer to the HEAD installation. Canonical form uses empty string internally; `"latest"` is only a display label.

---

## 2. `Arrow` Aggregate

The durable catalog entry. Keyed by `Namespace`. Owns the version inventory for that namespace — one aggregate, all installed versions.

```go
type Arrow struct {
    Namespace   Namespace
    Name        string          // display metadata — from the most recently resolved manifest
    Description string
    Version     string          // upstream software version of the most recently resolved manifest
    License     string
    Tags        []string
    Versions    map[string]ArrowVersion // key: version string ("latest", "v1.2.3", etc.)
    Removed     bool
}
```

Display metadata (`Name`, `Description`, `Version`, `License`, `Tags`) is stored at the namespace level from the most recently resolved manifest — not per-version. This is a pragmatic choice: display names rarely change between versions and the cost of staleness is low.

### `ArrowVersion`

Each entry in `Versions` represents one installed version.

```go
type ArrowVersion struct {
    CompiledTargets map[OS]ResolvedTarget // keyed by GOOS/GOARCH — output of SelectTarget
    InstalledAt     time.Time
    DirectInstall   bool                 // true if the user explicitly added this version;
                                         // false if installed automatically as a dependency
}
```

`CompiledTargets` is populated when the version is installed (`arrow.Add` + `SelectTarget`). A version entry with an empty `CompiledTargets` map means it was registered but not yet compiled — this should not occur in normal operation.

`DirectInstall` governs removal: `quiver remove` only removes versions where `DirectInstall: true`. Dependency-only versions (`DirectInstall: false`) are removed only by orphan detection during uninstall.

---

## 3. `ArrowManifest`

Raw, as parsed from YAML. Stored in the Vault for display and re-compilation. Targets are unflattened; Overrideable fields are intact.

```go
type ArrowManifest struct {
    Name        string
    Description string
    Version     string
    License     string
    URL         string
    Maintainers []Person
    Credits     []Person
    Tags        []string
    Variables   []Variable
    Netbridge   []PortDef
    Targets     map[string]Target // key: target key string ("linux/*", "_common", "*", etc.)
}
```

`ArrowManifest` is never executed directly. The app layer calls `SelectTarget` on it to produce `ResolvedTarget` for a specific OS.

### `Person`

```go
type Person struct {
    Name  string
    Email string
    URL   string
}
```

### `Target`

One entry in `Targets`. May be concrete (matched by OS at runtime) or abstract (`_`-prefixed, referenced only via `Base`).

```go
type Target struct {
    Base         string                        // parent target key; empty = no parent
    Requirements Requirement
    Tools        []ArrowRef                    // install-time tool dependencies
    Services     []ArrowRef                    // runtime service dependencies
    Exports      map[string]Overrideable[string]
    Lifecycle    TargetLifecycle
    Methods      map[string]Method
}

type TargetLifecycle struct {
    Install   []Step
    Execute   []Step
    Stop      []Step
    Uninstall []Step
}
```

### `ResolvedTarget`

The output of `SelectTarget(os)`. Fully flattened — `Base` chain resolved, Overrideable fields collapsed to concrete values for the given OS. This is what the runner, installer, and dep-checker read at runtime.

```go
type ResolvedTarget struct {
    Requirements Requirement
    Tools        []ArrowRef
    Services     []ArrowRef
    Exports      map[string]string  // Overrideable resolved; values are static strings (no ${VAR} tokens)
    Lifecycle    TargetLifecycle    // steps have Overrideable fields resolved
    Methods      map[string]Method  // steps have Overrideable fields resolved
}
```

---

## 4. `Step` Interface

Steps are the primitive execution unit. Used in `Target.Lifecycle`, `ResolvedTarget.Lifecycle`, and `Method.Steps`. Three authored types: `run`, `fetch`, `signal`. One synthetic type: `dependencies`.

```go
type Step interface {
    Type() StepType
    Title() string
    ExitOnFailure() bool
}
```

### `BasicStep`

Embedded into every concrete step type.

```go
type BasicStep struct {
    stepType      StepType
    exitOnFailure bool   // default true at construction
    title         string
}

func (bs BasicStep) Type() StepType      { return bs.stepType }
func (bs BasicStep) Title() string       { return bs.title }
func (bs BasicStep) ExitOnFailure() bool { return bs.exitOnFailure }
```

### Concrete step types

```go
type RunStep struct {
    BasicStep
    Command  Overrideable[string]
    Elevated Overrideable[bool]   // true = sudo on Linux/macOS, UAC runas on Windows
    Timeout  Overrideable[time.Duration]
}

type FetchStep struct {
    BasicStep
    URL      Overrideable[string]
    To       Overrideable[string]
    Checksum Overrideable[string] // optional; format: "<algorithm>:<hex-digest>" e.g. "sha256:abc123"
    Timeout  Overrideable[time.Duration]
}

type SignalStep struct {
    BasicStep
    Signal  Overrideable[SignalKind]
    Timeout Overrideable[time.Duration]
}

type DependenciesStep struct {
    BasicStep
}
```

### `SignalKind`

Cross-platform shutdown signal enum. Never raw POSIX signal names.

```go
type SignalKind string

const (
    SignalKindGraceful  SignalKind = "graceful"  // SIGTERM on Unix; Stop-Process on Windows
    SignalKindKill      SignalKind = "kill"       // SIGKILL on Unix; taskkill /F on Windows
    SignalKindInterrupt SignalKind = "interrupt"  // SIGINT on Unix; GenerateConsoleCtrlEvent on Windows
)
```

### `Overrideable[T]`

A value that may vary per `GOOS/GOARCH` within a glob target.

```go
type Overrideable[T any] struct {
    Default T
    OSArch  map[string]T // keys: exact GOOS/GOARCH, glob patterns, or "default"
}
```

After `SelectTarget` runs, all `Overrideable` fields in a `ResolvedTarget` have been collapsed — `Default` holds the resolved value and `OSArch` is empty.

### Constructors

```go
func NewRunStep(title string, command string, elevated bool, timeout time.Duration, exitOnFailure bool) RunStep
func NewFetchStep(title string, url string, to string, checksum string, timeout time.Duration, exitOnFailure bool) FetchStep
func NewSignalStep(title string, signal SignalKind, timeout time.Duration, exitOnFailure bool) SignalStep
func NewDependenciesStep(title string) DependenciesStep
```

`DependenciesStep` is synthetic — never authored in manifests. The app layer injects it as **Step 0** of every `_install` execution. The Wizard never receives it; the app layer manages Step 0 progress directly. See `deptree.md` §Call Site.

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

---

## 5. `ArrowRuntime` Aggregate

The volatile execution context for one installed version. Keyed by `ArrowRef` — `(namespace, version)`. Created lazily when `_install` begins for that version.

### `ArrowState`

```go
type ArrowState string

const (
    ArrowStateAbsent       ArrowState = "absent"
    ArrowStateInstalling   ArrowState = "installing"
    ArrowStateUpdating     ArrowState = "updating"
    ArrowStateReady        ArrowState = "ready"
    ArrowStateRunning      ArrowState = "running"
    ArrowStateStopping     ArrowState = "stopping"
    ArrowStateUninstalling ArrowState = "uninstalling"
    ArrowStateRemoved      ArrowState = "removed"
)
```

State machine:

```
nil ──[BeginExecution{_install}]──→ installing ──[EndExecution{success}]──→ ready
  ↑                                     │                                    ↑  │
  │                                     └── [EndExecution{failed|cancelled}] │  ├── [BeginExecution{_update}]──→ updating
  │                                         → absent                         │  │         ├── [EndExecution{success}]──→ ready
  │                                             │                            │  │         └── [EndExecution{failed}]───→ ready (kept current)
  │                                             │ [BeginExecution{_install}]  │  │
  │                                             └── → installing (retry)     │  └── [BeginExecution{_execute}]──→ running
  │                                                                          │              ↓ [MarkStopping]
  │                                                                          │          stopping ←─[BeginExecution{_stop}]─┐
  │                                                                          │              │                               │
  │                                                                          │              ├── [EndExecution{_stop}] ───→ ready
  │                                                                          │              └── [EndExecution{_execute}] (natural exit) ──→ ready
  │
ready ──[BeginExecution{_uninstall}]──→ uninstalling ──[EndExecution{success}]──→ removed
                                                    └── [EndExecution{failed}]──→ ready (rollback)
```

**`updating` key properties:**
- Only reachable from `ready` — a running service must be stopped before updating
- Failure transitions back to `ready`, not `absent` — the current installation is always preserved
- On success, the installation directory is updated in-place; the `ArrowVersion` entry is refreshed

```go
type ArrowRuntime struct {
    Ref        ArrowRef    // (namespace, version) — aggregate key
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
    Variables map[string]string
}
```

### `Return`

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

```go
type StepProgress struct {
    Index  int
    Status StepStatus
    Error  *string
    Step   Step // resolved at execution time — Overrideable fields already collapsed
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

## 6. `Method`

Developer-defined action gated by lifecycle state.

```go
type Method struct {
    AvailableIn []ArrowState // valid values: ArrowStateReady, ArrowStateRunning
    Steps       []Step
}
```

---

## 7. `Quiver` Aggregate

Unchanged. Keyed by `Namespace`.

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
    Maintainers []Person
    Tags        []string
    Media       QuiverMedia
    Arrows      []Namespace
}

type QuiverMedia struct {
    Icon   string
    Banner string
}
```

---

## 8. Supporting Types

| Type | Notes |
|------|-------|
| `Namespace` | `domain/namespace.go`. Aggregate ID for `Arrow` and `Quiver`. Parses `@ref` suffix for version extraction. |
| `ArrowRef` | `domain/arrow_ref.go`. Aggregate ID for `ArrowRuntime` and Vault entries. |
| `Variable` | `domain/variable.go`. Unchanged. |
| `Requirement` | `domain/requirement.go`. `OS []OS` field removed — platform coverage is expressed entirely through target keys, not requirements. Only `CpuCores`, `MemoryGB`, `DiskGB` remain. |
| `PortDef` | `domain/port.go`. Unchanged. |
| `SignalKind` | `domain/step.go`. Enum: `graceful`, `kill`, `interrupt`. |
| `Overrideable[T]` | `domain/overrideable.go`. Generic; used in step types and `Target.Exports`. |

---

## 9. Aggregate Coordination

`Arrow` and `ArrowRuntime` share the same `Namespace` as a conceptual grouping but use different keys:

- `Arrow` — keyed by `Namespace`. One aggregate per software namespace. Owns the version inventory.
- `ArrowRuntime` — keyed by `ArrowRef`. One aggregate per `(namespace, version)` pair. Owns the lifecycle state for that version.

The app layer coordinates between them. The aggregates have no knowledge of each other.

`quiver list` reads `Arrow` aggregates (grouped by namespace, with version inventory), then resolves each version's state from its `ArrowRuntime`.
