# Quiver — Subscriptions & Handlers

## Overview

Subscriptions are the reactive layer. When a command is sent and an event is emitted, registered handlers fire asynchronously. Asynx `Subscribe(pattern, handler)` matches event names against a regex — dot notation in event names enables precise targeting.

```go
type ProjectionHandler[T any] func(event Event[T]) error
```

Each `Event[T]` carries both states:

```go
type Event[T any] struct {
    Aggregate         T      // state after the command
    PreviousAggregate T      // state before the command
    EventName         string
    AggregateID       string
    Version           int64
}
```

`PreviousAggregate` is essential for coordinators — by the time some events fire, the field they need to inspect has already been cleared on `Aggregate`.

No read projections are built here. `Asynx[T].Get()` serves as the query infrastructure.

Two categories of handlers:
- **Coordinators** — OS-level execution orchestration and stop signalling
- **WebSocketHub** — real-time frontend feed

---

## Coordinators

### Execution handler — `Asynx[ArrowRuntime]`, pattern: `runtime\.Begin`

This handler lives in the **use case layer**, not in the Wizard. On `runtime.Begin`, the use case layer constructs an `ExecutionRequest` and calls `wizard.Execute()` in a goroutine. The Wizard has no knowledge of Asynx — it reports progress through a `StepReporter` callback interface that the use case layer implements by sending Asynx commands.

```
on runtime.Begin:
    req := buildExecutionRequest(event)
    reporter := &asynxStepReporter{...}

    go func() {
        err := wizard.Execute(ctx, req, reporter)
        // reporter already sent Advance commands per step
        asynxRuntime.Send(EndExecution{namespace})
    }()
```

See [wizard.md](wizard.md) for the full Wizard contract and `StepReporter` interface.

---

### `StopCoordinator` — `Asynx[ArrowRuntime]`, pattern: `runtime\.MarkStopping`

Signals the Wizard to cancel the currently running `_execute` goroutine. The Wizard holds a per-namespace `context.CancelFunc`; the coordinator fires the cancel, the `_execute` goroutine unwinds, and the use case layer sends `EndExecution`. The use case layer then begins a separate `_stop` execution with the stop lifecycle steps.

```go
// on runtime.MarkStopping
wizard.Cancel(event.Aggregate.Namespace)
```

The full stop sequence is documented in [wizard.md § Stop Flow](wizard.md#stop-flow--full-sequence).

---

## WebSocketHub

One hub, registered on all three Asynx instances. Connected clients join rooms keyed by `Namespace` — only events for namespaces they are watching are pushed.

### Runtime feed — `Asynx[ArrowRuntime]`, pattern: `^runtime\.`

Pushes execution progress to clients watching the affected namespace.

| Event | WebSocket message |
|-------|------------------|
| `runtime.Begin` | `execution.started { method, total_steps }` |
| `runtime.Advance` | `execution.step { index, status, error? }` |
| `runtime.RecordPID` | `execution.pid { pid }` |
| `runtime.MarkStopping` | `arrow.stopping { namespace }` |
| `runtime.End` | `execution.ended { method, success }` + `arrow.{state} { namespace }` |

`runtime.Advance` is the high-frequency message — fires twice per step. Clients use it to render live progress.

On `runtime.End`, the handler reads `event.Aggregate.State` to push the appropriate Arrow state message alongside `execution.ended` — `arrow.ready` after install or execute, `arrow.removed` after uninstall.

---

### State feed — `Asynx[Arrow]`, pattern: `^arrow\.`

Pushes Arrow catalog changes to all connected clients.

| Event | WebSocket message |
|-------|------------------|
| `arrow.Add` | `arrow.added { namespace, name }` |
| `arrow.UpdateManifest` | `arrow.updated { namespace, name }` |
| `arrow.Remove` | `arrow.removed { namespace }` |

---

### Catalog feed — `Asynx[Quiver]`, pattern: `^quiver\.`

Pushes Quiver catalog changes to all connected clients.

| Event | WebSocket message |
|-------|------------------|
| `quiver.Add` | `quiver.added { namespace, name }` |
| `quiver.UpdateManifest` | `quiver.updated { namespace, name }` |
| `quiver.Remove` | `quiver.removed { namespace }` |

---

## Summary

| # | Name | Asynx instance | Pattern | Category | Does |
|---|------|----------------|---------|----------|------|
| 1 | Execution handler | `Asynx[ArrowRuntime]` | `runtime\.Begin` | Coordinator | Use case layer calls `wizard.Execute`, translates callbacks to Asynx commands |
| 2 | `StopCoordinator` | `Asynx[ArrowRuntime]` | `runtime\.MarkStopping` | Coordinator | Calls `wizard.Cancel(namespace)`, use case layer coordinates `_stop` execution |
| 3 | `WebSocketHub` | `Asynx[ArrowRuntime]` | `^runtime\.` | WebSocket | Pushes execution progress + Arrow state to frontend |
| 4 | `WebSocketHub` | `Asynx[Arrow]` | `^arrow\.` | WebSocket | Pushes Arrow catalog changes to frontend |
| 5 | `WebSocketHub` | `Asynx[Quiver]` | `^quiver\.` | WebSocket | Pushes Quiver catalog changes to frontend |
