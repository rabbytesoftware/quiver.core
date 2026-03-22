# Quiver — Wizard Module

## Overview

The Wizard is the execution coordinator. It receives a namespace, a list of resolved steps, and resolved variables — then runs those steps sequentially, reporting progress through a callback interface. The Wizard has **no knowledge of Asynx** — it is a pure infrastructure module. The use case layer owns all Asynx interactions and translates Wizard callbacks into commands.

The Wizard owns two submodules that it uses to execute steps:
- **Runtime** — process spawn/manage (RunStep, SignalStep)
- **FetchNShare (FNS)** — file operations and HTTP download (FetchStep) — existing module at `internal/core/fns`

---

## Contract

```go
type Wizard struct {
    runtime *runtime.Runtime
    fns     fns.FNSInterface

    executions map[string]context.CancelFunc // namespace → cancel
    mu         sync.Mutex
}

func New(runtime *runtime.Runtime, fns fns.FNSInterface) *Wizard
```

### `Execute`

Blocking. Runs all steps in sequence. Returns when all steps complete, a step fails with `ExitOnFailure`, or the context is cancelled.

```go
func (w *Wizard) Execute(ctx context.Context, req ExecutionRequest, reporter StepReporter) error
```

The Wizard stores a derived context with cancel per namespace. This enables `Cancel(namespace)` to interrupt a running execution from the outside.

```go
func (w *Wizard) Execute(ctx context.Context, req ExecutionRequest, reporter StepReporter) error {
    ctx, cancel := context.WithCancel(ctx)

    w.mu.Lock()
    w.executions[req.Namespace.String()] = cancel
    w.mu.Unlock()

    defer func() {
        w.mu.Lock()
        delete(w.executions, req.Namespace.String())
        w.mu.Unlock()
        cancel()
    }()

    for i, step := range req.Steps {
        reporter.OnStepStarted(i)

        err := w.executeStep(ctx, req, step, reporter)

        if err != nil {
            reporter.OnStepFailed(i, err)
            if step.ExitOnFailure() {
                return &StepError{Index: i, Step: step, Err: err}
            }
            continue
        }

        reporter.OnStepCompleted(i)
    }

    return nil
}
```

### `Cancel`

Non-blocking. Fires the cancel function for the given namespace. The goroutine blocked in `Execute` receives the context cancellation and unwinds.

```go
func (w *Wizard) Cancel(namespace Namespace) {
    w.mu.Lock()
    defer w.mu.Unlock()

    if cancel, ok := w.executions[namespace.String()]; ok {
        cancel()
    }
}
```

---

## ExecutionRequest

Everything the Wizard needs to run an execution. Fully resolved — no `${VAR}` placeholders, no Asynx types.

```go
type ExecutionRequest struct {
    Namespace Namespace
    Method    string
    Variables map[string]string
    Steps     []Step
    WorkDir   string // ~/.quiver/arrows/{namespace}/
}
```

The use case layer constructs this from the `ArrowManifest` and resolved variables before calling `wizard.Execute`.

### Working Directory

Every Arrow gets its own working directory:

| Platform | Path |
|----------|------|
| Linux/macOS | `~/.quiver/arrows/{namespace}/` |
| Windows | `%USERPROFILE%\Documents\.quiver\arrows\{namespace}\` |

The use case layer resolves the platform-specific path and passes it as `WorkDir`. The Wizard does not resolve paths — it uses `WorkDir` as-is.

---

## StepReporter — Callback Interface

The use case layer implements this interface and translates each call into an Asynx command.

```go
type StepReporter interface {
    OnStepStarted(index int)
    OnStepCompleted(index int)
    OnStepFailed(index int, err error)
    OnPIDRecorded(pid int)
}
```

### Use case layer implementation

```go
// inside ArrowUseCases — implements wizard.StepReporter
type asynxStepReporter struct {
    namespace Namespace
    asynx     *asynx.Asynx[ArrowRuntime]
}

func (r *asynxStepReporter) OnStepStarted(index int) {
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index,
        ToStatus:       StepStatusRunning,
    })
}

func (r *asynxStepReporter) OnStepCompleted(index int) {
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index,
        ToStatus:       StepStatusCompleted,
    })
}

func (r *asynxStepReporter) OnStepFailed(index int, err error) {
    errStr := err.Error()
    r.asynx.Send(AdvanceStep{
        ArrowNamespace: r.namespace,
        StepIndex:      index,
        ToStatus:       StepStatusFailed,
        Error:          &errStr,
    })
}

func (r *asynxStepReporter) OnPIDRecorded(pid int) {
    r.asynx.Send(RecordPID{
        ArrowNamespace: r.namespace,
        PID:            pid,
    })
}
```

---

## Step Execution

Each step type dispatches to the appropriate submodule. Every step respects the context for cancellation.

### `executeStep` dispatcher

```go
func (w *Wizard) executeStep(ctx context.Context, req ExecutionRequest, step Step, reporter StepReporter) error {
    switch s := step.(type) {
    case RunStep:
        return w.executeRunStep(ctx, req, s, reporter)
    case FetchStep:
        return w.executeFetchStep(ctx, req, s)
    case SignalStep:
        return w.executeSignalStep(ctx, req, s)
    default:
        return &WizardError{Err: ErrUnknownStepType}
    }
}
```

### RunStep

Spawns a process via the Runtime submodule. Blocks until the process exits or the context is cancelled.

**PID reporting for `_execute`:** When the method is `_execute`, the last RunStep in the lifecycle is the long-running server process. After `Start`, the Wizard calls `reporter.OnPIDRecorded(pid)` so the use case layer can send `RecordPID`. The Wizard then blocks on `Wait` — the goroutine stays alive until the process exits naturally or is cancelled.

```go
func (w *Wizard) executeRunStep(ctx context.Context, req ExecutionRequest, step RunStep, reporter StepReporter) error {
    stepCtx := ctx
    if step.Timeout > 0 {
        var cancel context.CancelFunc
        stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
        defer cancel()
    }

    proc := w.runtime.Get(stepCtx, "sh", "-c", step.Command).
        WithWorkDir(req.WorkDir).
        WithEnv(req.Variables)

    built, err := proc.Build()
    if err != nil {
        return &WizardError{Err: err}
    }

    if err := built.Start(stepCtx); err != nil {
        return &WizardError{Err: err}
    }

    // Surface PID to the use case layer
    if pid := built.PID(); pid > 0 {
        reporter.OnPIDRecorded(pid)
    }

    // Blocks until process exits or context cancelled
    if err := built.Wait(stepCtx); err != nil {
        return &WizardError{Err: err}
    }

    return nil
}
```

### FetchStep

Downloads a file from a URL and saves it to the specified path using FetchNShare (`internal/core/fns`). Paths are relative to `WorkDir` by default.

```go
func (w *Wizard) executeFetchStep(ctx context.Context, req ExecutionRequest, step FetchStep) error {
    stepCtx := ctx
    if step.Timeout > 0 {
        var cancel context.CancelFunc
        stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
        defer cancel()
    }

    dest := step.To
    if !filepath.IsAbs(dest) {
        dest = filepath.Join(req.WorkDir, dest)
    }

    return w.fns.Download(stepCtx, step.URL, dest, nil)
}
```

### SignalStep

Sends an OS signal to the currently running PID for this namespace. Used in stop lifecycle steps to gracefully terminate a process.

```go
func (w *Wizard) executeSignalStep(ctx context.Context, req ExecutionRequest, step SignalStep) error {
    stepCtx := ctx
    if step.Timeout > 0 {
        var cancel context.CancelFunc
        stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
        defer cancel()
    }

    sig, err := parseSignal(step.Signal) // "SIGTERM" → syscall.SIGTERM
    if err != nil {
        return &WizardError{Err: err}
    }

    // The Runtime submodule exposes signal sending on tracked processes
    proc, err := w.runtime.GetByNamespace(req.Namespace.String())
    if err != nil {
        return &WizardError{Err: err}
    }

    if err := proc.Signal(sig); err != nil {
        return &WizardError{Err: err}
    }

    // If timeout is set, wait for the process to acknowledge the signal
    // (exit or respond). If timeout expires, the context cancels and we return.
    select {
    case <-stepCtx.Done():
        return &WizardError{Err: stepCtx.Err()}
    case <-proc.Done():
        return nil
    }
}
```

---

## Error Handling

The Wizard wraps all errors in domain-specific types so callers can distinguish Wizard failures from other errors.

```go
// WizardError wraps any infrastructure error that occurs during step execution.
type WizardError struct {
    Err error
}

func (e *WizardError) Error() string { return fmt.Sprintf("wizard: %v", e.Err) }
func (e *WizardError) Unwrap() error { return e.Err }

// StepError wraps a failure tied to a specific step. Returned from Execute
// when a step fails with ExitOnFailure.
type StepError struct {
    Index int
    Step  Step
    Err   error
}

func (e *StepError) Error() string {
    return fmt.Sprintf("wizard: step %d (%s) failed: %v", e.Index, e.Step.Title(), e.Err)
}
func (e *StepError) Unwrap() error { return e.Err }
```

### Sentinel errors

```go
var (
    ErrUnknownStepType  = errors.New("unknown step type")
    ErrNoProcess        = errors.New("no process found for namespace")
    ErrInvalidSignal    = errors.New("unrecognized signal name")
    ErrExecutionExists  = errors.New("execution already in progress for namespace")
)
```

### Error propagation flow

```
Wizard (infrastructure error)
  → wraps in WizardError or StepError
    → returns to use case layer
      → use case layer decides: retry, abort, report to user
```

The Wizard never escalates or retries on its own. It reports the exact error and the use case layer decides the response.

---

## Stop Flow — Full Sequence

The stop flow is the critical coordination path. Here is the complete sequence from user request to process termination:

```
1. User sends stop request → HTTP handler calls ArrowUseCases.Stop(namespace)

2. Use case layer sends MarkStopping command
   → ArrowRuntime.State = stopping
   → event "runtime.MarkStopping" emitted

3. StopCoordinator subscription fires
   → calls wizard.Cancel(namespace)
   → context cancelled for the _execute goroutine

4. Inside wizard.Execute (the _execute goroutine):
   → the blocked RunStep (long-running process) receives context cancellation
   → process is killed, Wait() returns with context.Canceled
   → step is reported as failed via reporter.OnStepFailed
   → Execute returns &StepError{...}

5. Use case layer receives the error from wizard.Execute
   → sends EndExecution command
   → ArrowRuntime.State = ready, CurrentExecution = nil

6. Use case layer sends BeginExecution with method="_stop" and stop lifecycle steps
   → ArrowRuntime.CurrentExecution = {Method: "_stop", Steps: [...]}

7. Use case layer calls wizard.Execute with the stop steps
   → Wizard runs stop steps (SignalStep, RunStep, etc.) sequentially
   → progress reported through StepReporter as normal

8. Stop steps complete (success or failure)
   → use case layer sends EndExecution
   → ArrowRuntime.State = ready, CurrentExecution = nil
```

Key points:
- The Wizard treats `_stop` identically to `_install`, `_execute`, or any method — it's just steps
- The use case layer is the coordinator between the two executions (`_execute` ending, `_stop` beginning)
- The Wizard never initiates a stop on its own — it only responds to context cancellation

---

## Submodule Interfaces

### Runtime (existing, needs refactoring)

The existing `runtime` package already provides process management. For the Wizard's needs, the key operations are:

```go
// What the Wizard uses from Runtime:
runtime.Get(ctx, command...) → Builder
runtime.GetByNamespace(namespace) → (Process, error)  // new — lookup by namespace tag

// Builder produces a Process:
builder.WithWorkDir(dir).WithEnv(env).Build() → (Process, error)

// Process interface (existing):
process.Start(ctx) error
process.Wait(ctx) error
process.Stop(ctx) error
process.Kill(ctx) error
process.Signal(sig os.Signal) error  // new — send arbitrary signal
process.PID() int                     // new — get OS pid
process.Done() <-chan struct{}        // new — closed when process exits
```

Changes needed to the existing Runtime for Wizard integration:
- `GetByNamespace` — processes tagged with namespace at creation for later lookup
- `Process.Signal` — send an arbitrary OS signal
- `Process.PID` — expose the OS PID
- `Process.Done` — channel that closes when the process exits

### FetchNShare (existing module — `internal/core/fns`)

File operations and HTTP download. The Wizard uses `FNSInterface` — primarily `Download(ctx, url, dst, progress)` for FetchStep execution. FNS already handles strategy selection (local vs remote) based on the path/URL prefix, parent directory creation, and context cancellation.

```go
// What the Wizard uses from FNS:
fns.Download(ctx, url, dst, progress) error
```

No changes needed to FNS for Wizard integration.

---

## Integration with Use Case Layer

The use case layer is the only caller. It owns Asynx, constructs `ExecutionRequest`, implements `StepReporter`, and calls `wizard.Execute` in a goroutine.

```go
// Inside ArrowUseCases — triggered when the user requests _install, _execute, etc.
func (uc *ArrowUseCases) beginExecution(ctx context.Context, namespace Namespace, method string) error {
    arrow, _ := uc.asynxArrow.Get(namespace.String())
    runtime, _ := uc.asynxRuntime.Get(namespace.String())

    // 1. Resolve variables, build step list
    steps := resolveSteps(arrow.Manifest, method, runtime.Variables)
    workDir := resolveWorkDir(namespace)

    // 2. Send BeginExecution command to Asynx
    uc.asynxRuntime.Send(BeginExecution{
        ArrowNamespace: namespace,
        Method:         method,
        Variables:      runtime.Variables,
        Steps:          toStepProgress(steps),
    })

    // 3. Build the request and reporter
    req := wizard.ExecutionRequest{
        Namespace: namespace,
        Method:    method,
        Variables: runtime.Variables,
        Steps:     steps,
        WorkDir:   workDir,
    }
    reporter := &asynxStepReporter{namespace: namespace, asynx: uc.asynxRuntime}

    // 4. Run in a goroutine — non-blocking for the caller
    go func() {
        err := uc.wizard.Execute(ctx, req, reporter)

        // 5. Always send EndExecution, regardless of success or failure
        uc.asynxRuntime.Send(EndExecution{ArrowNamespace: namespace})

        // 6. If this was _execute and it was cancelled (stop flow),
        //    the stop handler will begin the _stop execution
        if err != nil {
            uc.handleExecutionError(namespace, method, err)
        }
    }()

    return nil
}
```

---

## Summary

| Aspect | Decision |
|--------|----------|
| **Asynx awareness** | None — Wizard is pure infrastructure |
| **Progress reporting** | `StepReporter` callback interface |
| **Cancellation** | `context.Context` per namespace, `Cancel(namespace)` fires `cancelFunc()` |
| **Step execution** | Sequential, blocking — each step completes before the next begins |
| **Long-running processes** | Wizard goroutine blocks on `Wait()` until process exits or context cancels |
| **Stop flow** | Two separate executions: use case ends `_execute`, begins `_stop` |
| **Error handling** | `WizardError` / `StepError` wrappers — Wizard never retries or escalates |
| **Submodules** | Runtime (process), FetchNShare (file ops + HTTP download) |
| **Working directory** | `~/.quiver/arrows/{namespace}/` (platform-specific) |
