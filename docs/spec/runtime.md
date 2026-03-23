# Quiver — Runtime Module

## Overview

Runtime is a submodule of the Wizard (the execution coordinator). It handles OS process spawning, lifecycle management, and signal delivery. The Wizard calls into Runtime to execute RunStep and SignalStep.

Runtime is **pure infrastructure** — it has no knowledge of Asynx, namespaces, or domain concepts. It tracks processes by a deterministic UUID v5 derived from the OS PID and start timestamp. The Wizard owns the namespace-to-process association; Runtime just manages processes.

Runtime is **stateless in the domain sense**. It holds an ephemeral in-memory process table (`map[string]Process`) for bookkeeping — this is OS process tracking, not persisted domain state. When the application exits, the table is gone.

Three internal layers:
- **Runtime** — public facade, entry point for the Wizard
- **Manager** — thread-safe process table (register, unregister, lookup)
- **Process** — per-process lifecycle (start, wait, stop, kill, signal)

> **Note:** Code snippets in this document are pseudocode and do not reflect the final implementation.

---

## Contract — Runtime Facade

```go
type Runtime struct {
    manager *Manager
}

func New() *Runtime
```

No `os string` field. Platform-specific behavior is handled entirely by build tags at compile time — no runtime OS detection, no `detectOS()`, no `isSupportedOS()`.

### `Get`

Returns a builder for configuring and constructing a process. The builder holds the context and command; the caller chains configuration methods then calls `Build()`.

```go
func (r *Runtime) Get(ctx context.Context, command ...string) *Builder
```

### `GetByKey`

Looks up a tracked process by its deterministic UUID. Returns `ErrProcessNotFound` if the key is not in the process table.

```go
func (r *Runtime) GetByKey(key string) (Process, error)
```

This replaces both `GetByID` (random UUID-based) and `GetByNamespace` (removed — namespace resolution is the Wizard's responsibility). The Wizard stores the key returned by `Process.Key()` and uses it to call `GetByKey`. Key generation is internal to Runtime — the Wizard treats it as an opaque string.

### `Shutdown`

Graceful shutdown of all tracked processes. Iterates the process table, sends SIGTERM to each running process, waits up to the grace period, then escalates to SIGKILL for any survivors. Unregisters all processes from the table.

```go
func (r *Runtime) Shutdown(ctx context.Context) error
```

---

## Builder Pattern

The builder configures a process before construction. It is returned by `Runtime.Get()` and consumed by calling `Build()`.

```go
type Builder struct {
    ctx     context.Context
    manager *Manager
    config  *Config
}

func (b *Builder) WithWorkDir(dir string) *Builder
func (b *Builder) WithEnv(env map[string]string) *Builder
func (b *Builder) WithEnvVar(key, value string) *Builder
func (b *Builder) WithGracePeriod(d time.Duration) *Builder
func (b *Builder) WithBufferSize(size int) *Builder
func (b *Builder) Build() (Process, error)
```

### Configuration

- **`WithWorkDir`** — sets the process working directory.
- **`WithEnv`** — merges environment variables into the process environment.
- **`WithEnvVar`** — sets a single environment variable.
- **`WithGracePeriod`** — configures the SIGTERM-to-SIGKILL grace period for context cancellation. Default: 10 seconds. See [Context Cancellation](#context-cancellation--graceful-shutdown).
- **`WithBufferSize`** — sets the output channel buffer size.

No namespace concept on the builder. The Wizard associates the returned process with a namespace itself.

### `Build()`

Validates the config, constructs a platform-specific process via build tags, and returns it **unstarted and unregistered**. Registration happens at `Start()` because the deterministic key requires the OS PID.

```go
// builder_unix.go (//go:build darwin || linux)
func (b *Builder) Build() (Process, error) {
    if err := b.config.Validate(); err != nil {
        return nil, err
    }
    return NewUnixProcess(b.ctx, b.config, b.manager)
}

// builder_windows.go (//go:build windows)
func (b *Builder) Build() (Process, error) {
    if err := b.config.Validate(); err != nil {
        return nil, err
    }
    return NewWindowsProcess(b.ctx, b.config, b.manager)
}
```

Build tags guarantee the platform — no `switch` on an OS string. The builder no longer needs an `os` field.

### Config

```go
type Config struct {
    Command     []string
    WorkDir     string
    Env         map[string]string
    GracePeriod time.Duration // SIGTERM-to-SIGKILL grace period (default: 10s)
    BufferSize  int           // output channel buffer size
}
```

---

## Process Interface

The full public contract for process lifecycle.

```go
type Process interface {
    // Identity
    PID() int              // OS process ID, 0 before Start
    Key() string           // deterministic key, empty before Start

    // Lifecycle
    Start(ctx context.Context) error
    Wait(ctx context.Context) error
    Stop(ctx context.Context) error
    Kill(ctx context.Context) error
    Signal(sig os.Signal) error
    Close() error

    // State
    Status() Status
    ExitCode() int
    Done() <-chan struct{}

    // Output
    Output() string
    Error() string
    StreamOutput() <-chan string
    StreamError() <-chan string
}
```

### Identity

**`PID()`** — Returns `cmd.Process.Pid`. Returns 0 before `Start()`. The Wizard reports this via `StepReporter.OnPIDRecorded(pid)` so the use case layer can send the `RecordPID` command to the ArrowRuntime aggregate.

**`Key()`** — Returns the deterministic UUID v5. Empty string before `Start()`. See [Process Key](#process-key) for the full scheme.

### Lifecycle

**`Start(ctx)`** — Sets up stdout/stderr pipes, calls `cmd.Start()`, generates the key, registers with the Manager, launches output reader goroutines and the context-aware wait goroutine. Transitions status: `prepared → running`.

```go
func (p *BaseProcess) Start(ctx context.Context) error {
    // ... validate state == prepared ...
    // ... create pipes ...

    if err := p.cmd.Start(); err != nil {
        return fmt.Errorf("failed to start: %w", err)
    }

    p.startTime = time.Now().UnixNano()
    p.key = generateKey(p.cmd.Process.Pid, p.startTime) // deterministic UUID v5
    p.SetStatus(StatusRunning)

    // Register now that we have a key
    p.manager.Register(p)

    // ... launch output goroutines ...
    // ... launch context-aware wait goroutine ...

    return nil
}
```

**`Wait(ctx)`** — Blocks until `doneChan` closes or context cancels.

```go
func (p *BaseProcess) Wait(ctx context.Context) error {
    select {
    case <-p.doneChan:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**`Stop(ctx)`** — Sends SIGTERM (Unix) or TerminateProcess (Windows). Transitions to `stopping`. Waits for `doneChan` or context timeout.

**`Kill(ctx)`** — Sends SIGKILL. Transitions to `killing`. Waits for `doneChan` or context timeout.

**`Signal(sig)`** — Sends an arbitrary OS signal. Platform-specific:
- **Unix:** `cmd.Process.Signal(sig)` — supports all POSIX signals.
- **Windows:** SIGKILL (native `Kill()`) and SIGTERM (mapped to `TerminateProcess`) supported. Any other signal returns `ErrUnsupportedSignal`.

```go
// UnixProcess
func (p *UnixProcess) Signal(sig os.Signal) error {
    if p.cmd.Process == nil {
        return ErrNoProcess
    }
    return p.cmd.Process.Signal(sig)
}

// WindowsProcess
func (p *WindowsProcess) Signal(sig os.Signal) error {
    if p.cmd.Process == nil {
        return ErrNoProcess
    }
    switch sig {
    case syscall.SIGKILL:
        return p.cmd.Process.Kill()
    case syscall.SIGTERM:
        return p.cmd.Process.Kill() // TerminateProcess
    default:
        return ErrUnsupportedSignal
    }
}
```

**`Done()`** — Returns `<-chan struct{}` that closes when the process exits. Used by SignalStep to `select` between context timeout and process termination.

**`Close()`** — Closes output handler channels. Called after the process is done and the caller is finished reading output.

### State

**`Status()`** — Returns the current process status. Values: `prepared`, `running`, `stopping`, `killing`, `finished`.

**`ExitCode()`** — Returns the process exit code. -1 before the process exits.

### Output

Output handling is kept as-is. `output.Handler` buffers stdout/stderr and provides both accumulated (`Output()`, `Error()`) and streaming (`StreamOutput()`, `StreamError()`) access. Channel-based streaming enables future WebSocket/frontend integration.

---

## Process Key

Processes are identified by a **deterministic UUID v5** derived from the OS PID and start timestamp:

```go
// A fixed namespace UUID for Runtime process key generation.
// Generated once, hardcoded — never changes.
var runtimeNamespace = uuid.MustParse("a1b2c3d4-...")

func generateKey(pid int, startTimeNanos int64) string {
    seed := fmt.Sprintf("%d-%d", pid, startTimeNanos)
    return uuid.NewSHA1(runtimeNamespace, []byte(seed)).String()
}
```

```
Input:  PID=12345, startTimeNanos=1679500000000000000
Output: "f47ac10b-58cc-5372-8567-0e02b2c3d479" (deterministic — same input always produces the same UUID)
```

- **PID** comes from `cmd.Process.Pid`, available after `cmd.Start()`.
- **startTimeNanos** is `time.Now().UnixNano()` captured immediately after successful `cmd.Start()`.
- **UUID v5** (SHA-1 based, RFC 4122) produces a proper UUID format from a namespace + seed. Same seed always yields the same UUID — no randomness, no stored state.

### Why UUID v5 instead of random UUID v4?

Key generation is an internal Runtime concern — the Wizard receives a key from `Process.Key()` and stores it, but never constructs or interprets keys itself. UUID v5 standardizes the key format so that if a future platform doesn't use PIDs (e.g., a containerized runtime with a different process model), the `generateKey` implementation can change its seed inputs while still producing proper UUIDs. The external contract (`string` key in UUID format) stays stable regardless of how Runtime internally derives it.

### Why not PID alone?

PID recycling: after a process exits, the OS can reassign that PID to an unrelated process. On Linux under load this can happen quickly. The nanosecond timestamp disambiguates — the combination is unique for the lifetime of the application.

### Registration timing

The key is generated after `Start()`, so there is a brief window between `Build()` and `Start()` where the process has no key and is not registered in the Manager. This is acceptable because the Wizard calls `Start()` immediately after `Build()`.

---

## Context Cancellation — Graceful Shutdown

Go's default `exec.CommandContext` sends SIGKILL immediately when the context is cancelled. This is too aggressive for server processes that need cleanup time. Runtime overrides this behavior.

### Approach

Do **not** use `exec.CommandContext`. Use plain `exec.Command` and handle context cancellation manually in the wait goroutine:

1. Context cancelled → send **SIGTERM**
2. Start grace period timer
3. If process exits within grace period → clean exit
4. Grace period expires → escalate to **SIGKILL**

Grace period is configurable via `Builder.WithGracePeriod(d)`. Default: **10 seconds**.

```go
// Inside the wait goroutine (Unix):
go func() {
    waitDone := make(chan struct{})
    go func() {
        p.cmd.Wait()
        close(waitDone)
    }()

    select {
    case <-ctx.Done():
        // Context cancelled — begin graceful shutdown
        p.cmd.Process.Signal(syscall.SIGTERM)
        p.SetStatus(StatusStopping)

        select {
        case <-time.After(p.config.GracePeriod):
            // Grace period expired, escalate
            p.cmd.Process.Kill()
            p.SetStatus(StatusKilling)
            <-waitDone
        case <-waitDone:
            // Process exited within grace period
        }

    case <-waitDone:
        // Process exited normally
    }

    // Capture exit code, set status to finished, close doneChan
    p.finalize()
}()
```

### Edge Cases

- **Process exits during grace period** — no SIGKILL sent, clean exit.
- **Context cancelled after process already exited** — no-op, `doneChan` already closed.
- **Multiple context cancels** — idempotent. Only the first SIGTERM is sent.
- **Grace period of 0** — skip SIGTERM, go straight to SIGKILL (equivalent to default `exec.CommandContext` behavior).

### Windows

On Windows, SIGTERM maps to `TerminateProcess`, which is immediate and forceful. The grace period is effectively a no-op — the process terminates on the first signal. This is a known platform limitation.

---

## Platform Consolidation

### Unix: Darwin + Linux

`darwin.go` and `linux.go` are byte-for-byte identical — same `SysProcAttr{Setpgid: true}`, same SIGTERM/SIGKILL logic. They merge into a single `UnixProcess`:

```go
//go:build darwin || linux

type UnixProcess struct {
    *BaseProcess
}
```

`UnixProcess` sets `Setpgid: true` for process group isolation. This ensures that signals sent to the process also reach child processes spawned by shell commands (e.g., `sh -c "some-command"`).

### Windows

`WindowsProcess` stays separate. No process group isolation (not supported). `Stop()` and `Signal(SIGTERM)` both map to `TerminateProcess`.

### File Mapping

| Current | New |
|---------|-----|
| `process/darwin.go` | `process/unix.go` (`//go:build darwin \|\| linux`) |
| `process/linux.go` | removed (merged into unix.go) |
| `process/windows.go` | `process/windows.go` (unchanged build tag) |
| `builder/builder_darwin.go` | `builder/builder_unix.go` (`//go:build darwin \|\| linux`) |
| `builder/builder_linux.go` | removed (merged into builder_unix.go) |
| `builder/builder_windows.go` | `builder/builder_windows.go` (unchanged) |

---

## Manager (Internal)

The Manager is an internal component — not part of the public API the Wizard uses. It provides a thread-safe process table.

```go
type Manager struct {
    processes map[string]Process
    mu        sync.RWMutex
}

func (m *Manager) Register(proc Process)
func (m *Manager) Unregister(key string)
func (m *Manager) GetByKey(key string) (Process, error)
func (m *Manager) Count() int
```

- **`Register`** — adds a process to the table, keyed by `proc.Key()`.
- **`Unregister`** — removes a process from the table by key. Called during process cleanup.
- **`GetByKey`** — returns the process for the given key, or `ErrProcessNotFound`.
- **`Count`** — returns the number of tracked processes.

---

## Error Handling

### Sentinel Errors

```go
var (
    ErrProcessNotFound   = errors.New("process not found")
    ErrEmptyCommand      = errors.New("command cannot be empty")
    ErrInvalidState      = errors.New("invalid process state for operation")
    ErrNoProcess         = errors.New("no underlying OS process")
    ErrKillTimeout       = errors.New("timeout waiting for process to exit after kill")
    ErrUnsupportedSignal = errors.New("signal not supported on this platform")
)
```

`ErrInvalidState` covers all "wrong state" cases — attempting to start an already-running process, stopping a finished process, etc. The caller can check `process.Status()` to determine the actual state.

`ErrUnsupportedSignal` is returned by `WindowsProcess.Signal()` for signals other than SIGKILL and SIGTERM.

---

## Code Review — Current vs New

### Additions

| Item | Description |
|------|-------------|
| `Process.PID()` | Exposes OS PID from `cmd.Process.Pid` |
| `Process.Key()` | Deterministic UUID v5 from PID + startTime |
| `Process.Signal(sig)` | Send arbitrary OS signal |
| `Process.Done()` | Expose `doneChan` on the interface |
| `Builder.WithGracePeriod(d)` | Configure SIGTERM-to-SIGKILL grace period |
| `ErrUnsupportedSignal` | Sentinel for Windows signal limitation |
| `UnixProcess` | Consolidated darwin+linux process type |
| `builder_unix.go` | Consolidated darwin+linux builder |
| Graceful context cancellation | SIGTERM → grace period → SIGKILL (replaces Go's default SIGKILL-on-cancel) |

### Removals

| Item | Reason |
|------|--------|
| `Runtime.GetByID()` | Replaced by `GetByKey()` |
| `Runtime.ListAll()` | Wizard does not need bulk listing |
| `Runtime.ListByStatus()` | Wizard does not need bulk listing |
| `Runtime.Count()` | Wizard does not need this |
| `Runtime.StopAll()` | Wizard does not need bulk stop |
| `Runtime.KillAll()` | Wizard does not need bulk kill |
| `Runtime.CleanupFinished()` | Wizard does not need this |
| `Runtime.OS()` | Build tags eliminate runtime OS detection |
| `Runtime.os` field | Build tags eliminate this |
| `detectOS()` / `isSupportedOS()` | Build tags eliminate these |
| `Process.ID()` | Replaced by `Key()` |
| `DarwinProcess` | Merged into `UnixProcess` |
| `LinuxProcess` | Merged into `UnixProcess` |
| `process/darwin.go` | Merged into `process/unix.go` |
| `process/linux.go` | Merged into `process/unix.go` |
| `builder/builder_darwin.go` | Merged into `builder/builder_unix.go` |
| `builder/builder_linux.go` | Merged into `builder/builder_unix.go` |
| `Builder.WithTimeout()` | Wizard owns timeouts via context |
| `Builder.WithStopTimeout()` | Replaced by `WithGracePeriod` |
| `Builder.WithKillTimeout()` | Subsumed by graceful cancellation |
| `Config.Timeout` | Removed |
| `Config.StopTimeout` | Replaced by `GracePeriod` |
| `Config.KillTimeout` | Removed |
| `Manager.ListAll()` | Not needed |
| `Manager.ListByStatus()` | Not needed |
| `Manager.StopAll()` | Not needed |
| `Manager.KillAll()` | Not needed |
| `Manager.CleanupFinished()` | Not needed |
| `Manager.ShutdownAll()` | Shutdown logic moves to Runtime facade |
| `Manager.Clear()` | Test-only, removed |
| `ErrUnsupportedOS` | Build tags make this compile-time |
| `ErrAlreadyStarted` | Consolidated into `ErrInvalidState` |
| `ErrNotRunning` | Consolidated into `ErrInvalidState` |
| `ErrAlreadyFinished` | Consolidated into `ErrInvalidState` |
| `uuid.New()` (random v4) | Replaced by deterministic `uuid.NewSHA1()` (v5) |

### Changes

| Item | What Changed |
|------|-------------|
| `Manager.Register` | Keyed by `proc.Key()` instead of `proc.ID()` |
| `Manager.Get` | Renamed to `GetByKey` |
| `BaseProcess.id` | Replaced by `key string` (UUID v5) + `startTime int64` fields |
| `Config` | `GracePeriod` replaces `StopTimeout`/`KillTimeout`/`Timeout` |
| `Build()` | No longer switches on OS string; build tag guarantees the platform |
| Process registration | Moved from `Build()` to `Start()` (key requires PID) |
| `exec.CommandContext` | Replaced by `exec.Command` with manual context cancellation handling |

### Cross-Reference: wizard.md

The Wizard spec currently references `runtime.GetByNamespace(namespace)` in the SignalStep section and submodule interfaces section. With this design, the Wizard stores the process key (returned by `Process.Key()`) alongside the namespace, and calls `runtime.GetByKey(key)` when it needs to look up a process. Key generation is entirely internal to Runtime — the Wizard treats it as an opaque string. The wizard spec will need a corresponding update.

---

## Summary

| Aspect | Decision |
|--------|----------|
| **Role** | Wizard submodule — process spawn, lifecycle, signal delivery |
| **Asynx awareness** | None — pure infrastructure |
| **Domain state** | None — ephemeral in-memory process table only |
| **Process key** | Deterministic UUID v5 from PID + startTime, generated after Start |
| **Namespace tracking** | None — Wizard owns namespace-to-key mapping via ArrowRuntime aggregate |
| **Platform support** | `UnixProcess` (darwin+linux via build tags), `WindowsProcess` |
| **Context cancellation** | SIGTERM first, configurable grace period (default 10s), then SIGKILL |
| **Windows signals** | SIGTERM maps to TerminateProcess; other signals return `ErrUnsupportedSignal` |
| **API surface** | Minimal: `Get` (builder), `GetByKey`, `Shutdown` |
| **Output handling** | Kept — buffered + streaming for future frontend use |
| **UUID strategy** | Deterministic v5 (SHA-1) replaces random v4 — keeps `uuid` dependency, eliminates randomness |
