# Quiver — Wizard Engine

## Purpose

The Wizard is a step-execution coordinator. Given an arrow namespace, a method (`_install`, `_execute`, `_stop`, `_uninstall`, `_update`), a list of fully resolved steps, and a resolved variables map, the Wizard runs the steps sequentially in a background goroutine and reports progress to the caller through an event channel.

The Wizard is pure infrastructure: it has no knowledge of arrow lifecycle state, asynx aggregates, vault, or any application concept. It receives a `RunRequest`, returns an `Execution` handle, and emits events. The use case / app layer owns variable resolution, manifest interpretation, working-directory allocation, and translating Wizard events into asynx commands.

The package layout under `internal/engine/wizard/`:

| Path | Role |
|------|------|
| `wizard.go` | Public `Wizard` interface, `New` + `WithSandboxHome`, `Start`, `Probe`, `Shutdown`, `ProcessAlive`, `PathStatus`, `SetupPath`, and the `Plan` function |
| `internal/models/` | `RunRequest`, `Event`, `EventKind`, `Execution`, `ExecutionImpl` |
| `internal/step/` | `Handler[S]` generic interface, `Request` carrier |
| `internal/step/run/` | `RunStep` handler — spawns OS processes |
| `internal/step/download/` | `FetchStep` handler — HTTP download via `internal/core/fns` |
| `internal/step/signal/` | `SignalStep` handler — sends OS signals to a PID |
| `internal/step/dependencies/` | `DependenciesStep` handler — calls an injected `Executor` |
| `internal/step/extract/` | `ExtractStep` handler — unpacks an archive into a directory |
| `internal/step/portable/` | `PortableStep` handler — materializes an AppImage, `.dmg`, archive or bare executable as a Quiver-owned app; its `internal/install` places the files, writes `${WORKDIR}/.quiver-apps.json` and anchors paths to the workdir |
| `internal/step/expose/` | `Plan` (the synthetic `expose`/`unexpose` steps of a run) and the `ExposeStep`/`UnexposeStep` bridge to `internal/shelf`: applies a run's expose steps in one shelf pass and reports each entry's outcome; removes the entries owned by the run's workdir |
| `internal/shelf/` | Places and removes an arrow's `expose` entries on the host (CLI entries in `~/.quiver/bin`, desktop entries per OS) and reports/sets up `PATH`. Its `internal/{models,platform,fsguard,ownership,resolve,cli,desktop,pathenv,mocks}` hold the request types, host layout, path guards, entry ownership, `auto` resolution, the CLI and desktop placers and the `PATH` editor |
| `internal/unpack/` | Archive, AppImage and DMG extraction behind a safety `Guard` (path-escape, symlink, size and entry-count limits); shared by `extract` and `portable`. Its `internal/{archive,appimage,dmg,guard,models}` hold the formats and the guard, `mocks/` the fixture builders |
| `internal/workfs/` | Path containment (`Inside`), relocation and the rename-aside `Swap` shared by `portable`, `unpack` and the shelf |
| `internal/runtime/` | Process spawn/signal sub-engine (see `runtime.md`) |
| `internal/mocks/` | Test doubles |

The `runtime/` subdirectory has its own spec at `runtime.md`; this document describes only the wizard engine itself.

---

## Stateless Architecture

The Wizard holds no per-namespace state. Concretely, the `wizard` struct contains only:

| Field | Purpose |
|-------|---------|
| `dispatch` | Read-only `StepType → DispatchFn` table populated in `New` |
| `runtime` | The shared process sub-engine |
| `shutdownCtx` / `cancel` | A single root context cancelled by `Shutdown` to abort every active run |
| `wg` | Tracks active `Start` goroutines so `Shutdown` can wait for them |
| `done` | Closed once `wg` drains, signaling `Shutdown` completion |
| `mu` / `shutting` / `shutdownOnce` | Guards re-entrant or post-shutdown `Start` calls |

There is no map of `namespace → cancel`, no map of `namespace → process`, and no `Cancel(namespace)` method. Every `Start` call is independent — the wizard does not even check for duplicate namespaces.

### Why this matters

Crash recovery and concurrency become trivial. The wizard never has to reconcile in-memory state with persisted state, because there is no in-memory state to reconcile. After a crash and restart, the wizard begins life empty; the app layer is responsible for re-driving any executions that were transient at the time of the crash, and for detaching any process that survived the crash via PID lookup.

### Cancel semantics under the stateless model

The wizard exposes no cancel verb. Cancellation is delivered through one of two contexts wired into every running step:

| Source | Effect |
|--------|--------|
| `ctx` passed to `Start(ctx, req)` | When the caller cancels its context, `Start` derives a `runCtx` from it; cancellation propagates through every step handler (timeouts, `proc.Wait`, HTTP download, signal dispatch) |
| `Shutdown(ctx)` | Cancels the wizard-level `shutdownCtx`; via `context.AfterFunc` this also cancels every active `runCtx`, abandoning all in-flight executions |

To stop a running `_execute`, the app layer cancels the per-execution context (or invokes `Shutdown` for a wholesale teardown). The actual graceful-stop semantics — sending `SIGTERM`/`SIGKILL` in a sequence — live in the arrow's `_stop` lifecycle steps, which the app layer dispatches as a separate `Start` call with its own `RunRequest` containing `SignalStep`s targeting the previously recorded PID.

---

## Public Interface

### Wizard

| Method | Behavior |
|--------|----------|
| `Start(ctx, req) Execution` | Launches a goroutine to execute `req.Steps` sequentially; returns the `Execution` handle immediately |
| `Shutdown(ctx) error` | Cancels every active execution's root context, waits (bounded by `ctx`) for goroutines to drain, returns `nil` on success or `ctx.Err()` on timeout |
| `ProcessAlive(pid) bool` | Reports whether the OS process with the given PID is currently running; delegated to the runtime sub-engine. Used by app-layer crash recovery to decide between "detach to running process" and "recover interrupted state" |
| `PathStatus(ctx) (PathStatus, error)` | Reports whether `~/.quiver/bin` is on `PATH` and which shell files configure it; backs `GET /v0/system/path` |
| `SetupPath(ctx) (PathStatus, error)` | Appends `~/.quiver/bin` to the user's `PATH` (shell rc files, or the user `Path` on Windows); runs only on explicit user action (`POST /v0/system/path`), never on its own |

`Shutdown` is idempotent (`sync.Once`). After `Shutdown` is invoked, any further `Start` call returns an `Execution` that is immediately finished with `ExecutionOutcomeCancelled`.

### Constructor

`New(depExec stepdeps.Executor, extractMaxBytes int64, opts ...Option) (Wizard, error)`

The first argument is the dependency-step executor function. Passing `nil` makes every `DependenciesStep` a no-op (used in tests and in lifecycles where dependency resolution is delegated outside the wizard). The second is the uncompressed-size cap the `extract` and `portable` handlers enforce (the engine container passes `arrows.extract_max_bytes`). `WithSandboxHome(home)` keeps every exposed entry and the `PATH` the wizard reports inside `home` instead of the user's real home; the engine container passes it whenever it runs under `WithHomeDir`, so tests never touch the real `~/.quiver/bin`, desktop entries or `/Applications`. The constructor builds the runtime sub-engine via `runtime.New()` (which fails with `ErrUnsupportedOS` outside `darwin`/`linux`/`windows`) and the shelf, then registers the built-in handlers in the dispatch table.

### RunRequest

| Field | Meaning |
|-------|---------|
| `Namespace` | Arrow namespace (used as the `NSKey` in step requests) |
| `Method` | Lifecycle method name (carried through but not interpreted) |
| `Variables` | Fully resolved environment variables; passed verbatim to handlers |
| `Steps` | Resolved domain steps (see `manifests/v0/arrow.md`) |
| `WorkDir` | Absolute working directory for processes and relative `FetchStep` destinations; allocated by `vault` |
| `PID` | Pre-existing OS PID. Non-zero only during crash recovery, when a detached process survived a restart and `SignalStep`s in `_stop` need to target it directly |

The wizard does not look up `WorkDir` or resolve variables — both arrive ready to use. Variable resolution layering happens in the app layer (`internal/app/repositories/runtime/internal/assembler`), not here.

### Execution

The handle returned from `Start` exposes three accessors.

| Accessor | Description |
|----------|-------------|
| `Events() <-chan Event` | Buffered channel (capacity 16); all step events arrive here, terminated by an `EventKindEnded` event followed by channel close. `Emit` is non-blocking — events dropped on a full buffer are silently lost |
| `Done() <-chan struct{}` | Closed when the execution finishes. `Outcome()` is valid after this fires |
| `Outcome() ExecutionOutcome` | Final outcome (`success`, `failed`, `cancelled`); set under a mutex before the channels close |

Because `Emit` is non-blocking, `EventKindEnded` may be dropped if the consumer falls behind. `Outcome()` is the authoritative completion signal.

---

## Events

Five event kinds, all carried in a single `Event` struct.

| Kind | Fields populated | When emitted |
|------|------------------|--------------|
| `step.started` | `StepIndex` | Before each step's handler runs |
| `step.completed` | `StepIndex` | After a step's handler returns `nil` |
| `step.failed` | `StepIndex`, `Err` | After a step's handler returns a non-nil error and the context is not yet cancelled |
| `pid` | `PID` | Emitted by the run handler immediately after `runtime.Start` returns a process; carries the OS PID |
| `ended` | `Outcome` | Best-effort terminal event emitted by `Finish` before channels close |

The app layer subscribes to these via `drainExecution` in `internal/app/repositories/runtime/internal/hooks.go`, translating each into an asynx command:

| Wizard event | Asynx command |
|--------------|---------------|
| `step.started` | `AdvanceStep{ToStatus: running}` |
| `step.completed` | `AdvanceStep{ToStatus: completed}` |
| `step.failed` | `AdvanceStep{ToStatus: failed, Error: …}` |
| `pid` | `RecordPID{PID: …}` |
| `ended` | (loop exits; `EndExecution{Outcome: exec.Outcome()}` follows) |

The Wizard does not call asynx, never knows about step indexing offsets, and never edits aggregate state — the hook layer owns that translation.

---

## Step Types

The dispatch table is fixed at construction time. Seven step types map to seven handlers, and `expose` steps are run by the wizard loop itself (see [Exposure](#exposure)); an unknown step type returns `ErrUnknownStepType` and is reported as `step.failed` (treated as a normal step failure, honoring the step's `ExitOnFailure`).

| Step type | Handler | Description |
|-----------|---------|-------------|
| `run` | `internal/step/run` | Spawns a shell-wrapped command; emits `EventKindPID` after start; blocks on `Wait`; non-zero exit returns `ErrNonZeroExit` |
| `fetch` | `internal/step/download` | Downloads URL → `WorkDir`-relative or absolute destination via `internal/core/fns`; expands `${VAR}` references using `req.Vars` |
| `signal` | `internal/step/signal` | Sends a `SignalKind` (graceful/kill/interrupt) directly to `req.PID`; returns `ErrNoProcess` if `PID <= 0` |
| `dependencies` | `internal/step/dependencies` | Calls the `Executor` injected at `New`; if `nil`, a no-op |
| `extract` | `internal/step/extract` | Unpacks a tar (plain or `.gz`/`.xz`/`.bz2`/`.zst`), zip or single compressed file into a `WorkDir`-anchored directory through `internal/unpack`; refuses an AppImage or `.dmg` with `ErrPortableFormat`, pointing at `portable` |
| `portable` | `internal/step/portable` | Installs an AppImage, `.dmg`, archive or bare executable as a Quiver-owned app (staging directory, ownership marker, swap with rollback), records the apps it produced in `${WORKDIR}/.quiver-apps.json`, then removes the source when it lies inside the workdir |
| `expose` | `internal/step/expose` (batch) | One step per `expose` entry of the target; see [Exposure](#exposure) |
| `unexpose` | `internal/step/expose` | Removes the entries owned by `req.WorkDir`; see [Exposure](#exposure) |

### Exposure

Placing an arrow's `expose` entries on the host is a side effect of running its lifecycle, so it happens inside the wizard run. The wizard plans the steps itself: `wizard.Plan(method, arrow, os, steps)`, which the runtime repository's `plannedAssembler` applies to every assembled run before the run is recorded, so the added steps are recorded like any other. For a target that declares `expose` entries, `_install` and `_update` end with one `expose` step per entry (desktop entries first, then CLI, in declaration order; title `Expose <kind> <name>`), and `_uninstall` starts with one `unexpose` step (`Remove exposed entries`). Neither is ever fatal (`ExitOnFailure` is false).

- **Expose.** When the loop reaches the run's contiguous `expose` steps it applies them in one shelf pass — a macOS `.app` bundle moved out of the workdir relocates CLI entries pointing into it, and the pass prunes the entries of the same bare namespace it did not place — then reports every step on its own: `step.completed` when the entry was placed (or an `auto` entry resolved to nothing), `step.failed` with an error wrapping `ErrRefused` when the shelf declined it (a name owned by another namespace or by the user, a target outside the workdir, …). A refusal fails its step, never the run. Because the steps come last, a fatal failure earlier in the run leaves nothing exposed.
- **Unexpose.** Removes only the entries that point into this run's workdir, plus entries of the same bare namespace whose workdir no longer exists. Ownership lives on each entry — the symlink target, the `X-Quiver-Namespace`/`X-Quiver-Workdir` lines of a `.desktop` file, the `REM quiver:`/`REM quiver-workdir:` lines of a `.cmd` shim, the `quiver:<bare>|<workdir>` shortcut description in the Start Menu folder, the `net.quiver.namespace` xattr on a macOS bundle — so uninstalling `bat@v1` never removes an entry that `bat@v2` took over. Taking over is by bare namespace: applying `bat@v2` replaces `bat@v1`'s entry of the same name.

The results travel only as the run's step progress; nothing outside the wizard knows the shelf exists. `PathStatus`/`SetupPath` are the one other door into it.

### Step Request

Every handler receives a `step.Request` derived from the `RunRequest` plus per-execution context.

| Field | Source |
|-------|--------|
| `NSKey` | `req.Namespace.String()` |
| `WorkDir` | `req.WorkDir` |
| `Vars` | `req.Variables` |
| `OSArch` | `runtime.GOOS + "/" + runtime.GOARCH`, computed per call; used to resolve `Overrideable[T]` step fields to their platform-specific value |
| `PID` | `req.PID` (pre-existing PID for recovery) |
| `Emit` | The `Execution.Emit` function — handlers use it to push events mid-step (currently only the run handler emits `pid`) |

### Per-step timeouts

`RunStep`, `FetchStep`, `SignalStep`, `ExtractStep`, and `PortableStep` each carry an `Overrideable[string]` `Timeout` field. When non-empty, the handler resolves it for the current OS, parses it as a Go `time.Duration`, and derives a `context.WithTimeout` from the parent context. The fetch handler additionally disables the underlying HTTP client's default timeout (`config.WithTimeout(0)`) so the wrapping context deadline becomes the sole authority — preventing the 30s default from firing before a longer step timeout takes effect.

### Step type / handler dispatch

```mermaid
flowchart LR
  A["Start(ctx, req)"] --> B["runSteps loop"]
  B --> C{ctx cancelled?}
  C -->|yes| Z[ExecutionOutcomeCancelled]
  C -->|no| D[emit step.started]
  D --> E[executeStep]
  E --> F{dispatch table}
  F -->|run| RUN[runtime.Start + Wait + ExitCode]
  F -->|fetch| FET[fns.Download]
  F -->|signal| SIG[runtime.SignalPID]
  F -->|dependencies| DEP[Executor func]
  F -->|extract| EXT[unpack archive via Guard]
  F -->|portable| POR[install + record apps]
  F -->|unexpose| UNX[shelf.Remove workdir]
  F -->|unknown| UNK[ErrUnknownStepType]
  RUN --> G{err?}
  FET --> G
  SIG --> G
  DEP --> G
  EXT --> G
  POR --> G
  UNX --> G
  UNK --> G
  G -->|nil| H[emit step.completed] --> B
  G -->|err & ctx cancelled| Z
  G -->|err & !ExitOnFailure| I[emit step.failed] --> B
  G -->|err & ExitOnFailure| J[emit step.failed] --> K[ExecutionOutcomeFailed]
```

---

## Variable Resolution Layering

The wizard receives steps already resolved for the host OS — the app layer's assembler walks the manifest, picks the platform target, applies variables, and produces a flat `[]step.Step`. The wizard performs only the small remaining resolutions:

| Resolution | Where it happens |
|------------|------------------|
| Manifest → flat step list (target selection, variable defaults) | App layer (`assembler`) before `Start` |
| Variable map injected into process env | `RunStep` handler — sets `config.Env = req.Vars` |
| `${VAR}` placeholders in fetch URL/destination | `FetchStep` handler — `os.Expand(..., req.Vars)` |
| `Overrideable[T]` per-OS field selection | Each handler — `field.Resolve(req.OSArch.String())` |

The wizard never touches the manifest schema, never reads from vault, and never consults configuration. Everything it needs is in the `RunRequest`.

---

## Execution Flow — Sequence

```mermaid
sequenceDiagram
  participant App as App layer (runtime repo)
  participant Wiz as wizard.Wizard
  participant Exec as Execution
  participant H as Step handler
  participant RT as runtime sub-engine

  App->>Wiz: New(depExec)
  Wiz->>RT: runtime.New()
  RT-->>Wiz: Runtime
  Wiz->>Wiz: register dispatch[Run/Fetch/Signal/Deps/Extract/Portable]
  Wiz-->>App: Wizard

  App->>Wiz: Start(ctx, req)
  Wiz->>Exec: NewExecution()
  Wiz->>Wiz: spawn goroutine (runCtx tied to ctx + shutdownCtx)
  Wiz-->>App: Execution

  loop for each step in req.Steps
    Wiz->>Exec: Emit(step.started, i)
    Wiz->>H: dispatch[s.Type()](runCtx, stepReq, s)
    H->>RT: (Run) Start + Wait
    RT-->>H: Process
    H->>Exec: Emit(pid, P)
    H-->>Wiz: nil | err
    alt err == nil
      Wiz->>Exec: Emit(step.completed, i)
    else err != nil and runCtx.Err() != nil
      Wiz-->>Wiz: outcome = Cancelled, break
    else err != nil
      Wiz->>Exec: Emit(step.failed, i, err)
      alt s.ExitOnFailure()
        Wiz-->>Wiz: outcome = Failed, break
      else
        Note over Wiz: continue loop
      end
    end
  end

  Wiz->>Exec: Finish(outcome)
  Exec->>Exec: emit ended (best-effort) + close channels
  App-->>Wiz: drain Events() loop
  App->>App: send EndExecution{outcome}
```

---

## Cancel / Stop Flow

The wizard has no API for surgical cancellation. Two cancellation paths exist, both tied to contexts.

### Per-execution cancel via caller's context

```mermaid
sequenceDiagram
  participant App as App layer
  participant Wiz as wizard
  participant Exec as Execution
  participant H as Active step handler

  App->>Wiz: Start(ctx, req)
  Wiz-->>App: Execution
  Note over Wiz: runCtx = WithCancel(ctx) + AfterFunc(shutdownCtx)
  par step is running
    Wiz->>H: handler(runCtx, ...)
  and external cancel
    App->>App: cancel(ctx)
  end
  ctx-->>Wiz: cancellation propagates to runCtx
  runCtx-->>H: Done
  H-->>Wiz: error (likely context.Canceled)
  Wiz->>Wiz: detect runCtx.Err() != nil
  Wiz->>Exec: Finish(Cancelled)
  Note over Wiz,Exec: no step.failed emitted when ctx already cancelled
```

The `runSteps` loop checks `ctx.Err()` both before each iteration (skipping unstarted steps) and after each handler returns (suppressing a stale `step.failed` event in favor of the cancel outcome). The final `runSteps` return also re-checks `ctx.Err()` to convert a "success" path into `Cancelled` if the cancel landed mid-final-step.

### Graceful stop of a running arrow

For a managed arrow, "stop" is not a wizard concern — it is a separate `Start` call from the app layer with `_stop` lifecycle steps:

```mermaid
sequenceDiagram
  participant User
  participant App as App layer
  participant WizExec as Wizard (_execute run)
  participant WizStop as Wizard (_stop run)
  participant Proc as OS process

  User->>App: stop(namespace)
  App->>App: cancel _execute context
  WizExec->>Proc: runCtx cancelled, runtime kills process
  WizExec-->>App: Execution drains, outcome=Cancelled
  App->>App: send EndExecution{Cancelled}
  App->>App: assemble _stop steps (SignalSteps + cleanup)
  App->>WizStop: Start(ctx, RunRequest{Method:"_stop", PID: lastPID, Steps:[...]})
  WizStop->>Proc: SignalStep → SignalPID(SIGTERM/SIGKILL/SIGINT)
  WizStop-->>App: Execution drains, outcome=Success
```

`SignalStep` does not consult any registry — it operates on `req.PID`, which the app layer carries forward from the previous `_execute`'s `RecordPID` event. This is what makes the wizard truly stateless across executions.

### Process registry — none

There is no `processKeys` or `executions` map. The runtime sub-engine's `SignalPID` is a pass-through to OS-level `kill(pid, sig)`. The "process registry" of the original PR #114 spec was removed by PR #155.

---

## Crash Recovery

The wizard contributes two primitives to crash recovery; the app layer drives the actual reconciliation.

### Wizard primitives

| Primitive | Behavior |
|-----------|----------|
| `Wizard.ProcessAlive(pid)` | Forwards to `runtime.IsAlive(pid)`. On Unix this performs a `kill(pid, 0)` probe. On Windows it returns `false` (safe default — assume dead, force interrupted recovery) |
| `RunRequest.PID` | Optional pre-existing PID. Non-zero values flow through to handlers (notably `SignalStep`) so a freshly constructed wizard can act on a process that survived a crash without ever having spawned it |

### App-layer flow on startup

```mermaid
flowchart TD
  S[runtime repo Start] --> L[listArrows]
  L --> P[for each persisted namespace<br/>preload aggregate]
  P --> ST{aggregate.State}
  ST -->|Running| RA[recoverRunning]
  ST -->|Installing<br/>Uninstalling<br/>Updating<br/>Stopping<br/>Draining| RI[RecoverInterrupted command]
  ST -->|Absent/Ready/Detached/Removed/Outdated| OK[no action]
  RA --> CHK{Execution.PID > 0<br/>and<br/>wizard.ProcessAlive PID?}
  CHK -->|yes| DET[RecordDetached command<br/>→ state=Detached]
  CHK -->|no| RI
  RI --> RST[transient state reset<br/>→ Ready or Absent]
```

The wizard itself starts empty; `RecoverTransients` (in `internal/app/repositories/runtime/internal/recovery.go`) walks the persisted aggregates and either:

- **Detaches** — if `Execution.PID > 0` and the OS still has that process, the aggregate is marked `Detached` (the process keeps running unmanaged; the wizard remains uninvolved).
- **Recovers interrupted** — for any namespace stuck in a transient state (`Installing`/`Uninstalling`/`Updating`/`Stopping`/`Draining`, or `Running` without a live PID), an `RecoverInterrupted` command resets the aggregate back to a stable state, leaving no in-flight execution for the wizard to reconcile.

The PID is persisted through the `RecordPID` asynx command (sent by `drainExecution` when the wizard emits `EventKindPID`). On the next boot, that field is the only bridge from the previous wizard generation to the new one.

### Wizard stateless refactor consequences

| Before PR #155 | After PR #155 |
|----------------|---------------|
| Wizard held `executions map[ns]CancelFunc` | No map; cancel via caller's `ctx` |
| Wizard held `processKeys map[ns]string` | No map; `SignalStep` reads `req.PID` |
| `Cancel(namespace)` method on Wizard | Removed; no method |
| Recovery had to reconcile in-memory wizard state with persisted state | Wizard starts empty; recovery only touches persisted aggregates |
| `SignalStep` looked up a `Process` handle by key | `SignalPID` is a stateless OS-level call |

---

## Shutdown

`Shutdown(ctx)` is the only wizard-wide control verb.

```mermaid
sequenceDiagram
  participant App
  participant Wiz as wizard
  participant Exec1 as Active Execution A
  participant Exec2 as Active Execution B

  App->>Wiz: Shutdown(ctx)
  Wiz->>Wiz: shutdownOnce: shutting=true; cancel(shutdownCtx)
  par AfterFunc on Exec A
    Wiz->>Exec1: cancel runCtx
    Exec1-->>Exec1: handlers unwind, Finish(Cancelled)
  and AfterFunc on Exec B
    Wiz->>Exec2: cancel runCtx
    Exec2-->>Exec2: handlers unwind, Finish(Cancelled)
  end
  Wiz->>Wiz: wg.Wait → close(done)
  alt ctx not expired
    Wiz-->>App: nil
  else ctx expired first
    Wiz-->>App: ctx.Err()
    Note over Wiz: cleanup continues in background
  end
```

After the first `Shutdown`, the `shutting` flag short-circuits new `Start` calls — they receive an `Execution` already finished with `Cancelled`, no goroutine is spawned, and no handler runs.

---

## Error Handling

The wizard returns no domain-wrapped error type. Step handlers return raw errors (sentinels where useful, e.g. `ErrNonZeroExit`, `ErrNoProcess`, `ErrInvalidSignal`); the wizard delivers them verbatim in the `step.failed` event's `Err` field. The app layer is responsible for any logging, classification, or user-facing translation.

The sentinels exposed at the public boundary are `ErrUnknownStepType` (re-exported from `internal/models`), surfaced when the dispatch table has no entry for a step's type, `ErrShuttingDown` and `ErrVacuousProbe`. A refused `expose` entry's `step.failed` error wraps `internal/step/expose.ErrRefused`.

---

## Cross-References

| Topic | Spec |
|-------|------|
| Process spawn, signaling, OS abstraction | [`runtime.md`](runtime.md) |
| Step types, manifest grammar, lifecycle methods | [`manifests/v0/arrow.md`](manifests/v0/arrow.md) |
| Use-case orchestration, assembler, hook drainage | [`usecases.md`](usecases.md) |
| Asynx command surface (`BeginExecution`, `AdvanceStep`, `RecordPID`, `EndExecution`) | [`commands.md`](commands.md) |
| Subscription topology (`runtime.begun.*`) and crash recovery wiring | [`subscriptions.md`](subscriptions.md) |
