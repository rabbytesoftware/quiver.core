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
- **Coordinators** — stop signalling and shutdown lifecycle
- **WebSocketHub** — real-time frontend feed

---

## Coordinators

Execution orchestration is **not** a subscription. The use case layer's `beginExecution()` method sends the `BeginExecution` command and directly calls `wizard.Execute()` in a goroutine — see [wizard.md § Integration](wizard.md#integration-with-use-case-layer) for the full flow. Subscriptions handle only reactive coordination (stop signalling) and shutdown.

### `StopCoordinator` — `Asynx[ArrowRuntime]`, pattern: `runtime\.MarkStopping`

Signals the Wizard to cancel the currently running `_execute` goroutine. The Wizard holds a per-namespace `context.CancelFunc`; the coordinator fires the cancel, the `_execute` goroutine unwinds, and the use case layer sends `EndExecution`. The use case layer then begins a separate `_stop` execution with the stop lifecycle steps.

```go
// on runtime.MarkStopping
wizard.Cancel(event.Aggregate.Namespace)
```

The full stop sequence is documented in [wizard.md § Stop Flow](wizard.md#stop-flow--full-sequence).

---

### Shutdown — App Layer SIGTERM Handler

Not a subscription — the app layer's shutdown hook (SIGTERM/SIGINT handler) calls `runtime.Shutdown(ctx)` to gracefully terminate all tracked processes. See [runtime.md § Shutdown](runtime.md#shutdown) for the SIGTERM → grace period → SIGKILL sequence.

---

## WebSocketHub

One hub, registered on all three Asynx instances. Clients connect to resource-scoped WebSocket endpoints — the URL is the subscription. The hub pushes the full versioned DTO (same shape as REST responses) on every event.

See [websocket.md](websocket.md) for the complete WebSocket protocol spec — endpoints, DTO shapes, event-to-push mapping, and connection lifecycle.

### Runtime feed — `Asynx[ArrowRuntime]`, pattern: `^runtime\.`

Pushes ArrowRuntime DTO to `/v1/arrow.runtime` (global) and `/v1/arrow.runtime/{namespace}` (scoped).

| Event | Push |
|-------|------|
| `runtime.Begin` | ArrowRuntime DTO |
| `runtime.Advance` | ArrowRuntime DTO |
| `runtime.MarkStopping` | ArrowRuntime DTO |
| `runtime.End` | ArrowRuntime DTO |

`runtime.Advance` is the high-frequency message — fires twice per step. Runtime events push **only** on ArrowRuntime channels, not on Arrow catalog channels.

---

### State feed — `Asynx[Arrow]`, pattern: `^arrow\.`

Pushes Arrow DTO to `/v1/arrow` (global) and `/v1/arrow/{namespace}` (scoped).

| Event | Push |
|-------|------|
| `arrow.Add` | Arrow DTO |
| `arrow.UpdateManifest` | Arrow DTO |
| `arrow.Remove` | Arrow DTO |

---

### Catalog feed — `Asynx[Quiver]`, pattern: `^quiver\.`

Pushes Quiver DTO to `/v1/quiver` (global) and `/v1/quiver/{namespace}` (scoped).

| Event | Push |
|-------|------|
| `quiver.Add` | Quiver DTO |
| `quiver.UpdateManifest` | Quiver DTO |
| `quiver.Remove` | Quiver DTO |

---

## Summary

| # | Name | Asynx instance | Pattern | Category | Does |
|---|------|----------------|---------|----------|------|
| 1 | `StopCoordinator` | `Asynx[ArrowRuntime]` | `runtime\.MarkStopping` | Coordinator | Calls `wizard.Cancel(namespace)`, use case layer coordinates `_stop` execution |
| 2 | `WebSocketHub` | `Asynx[ArrowRuntime]` | `^runtime\.` | WebSocket | Pushes ArrowRuntime DTO to `/v1/arrow.runtime` channels |
| 3 | `WebSocketHub` | `Asynx[Arrow]` | `^arrow\.` | WebSocket | Pushes Arrow DTO to `/v1/arrow` channels |
| 4 | `WebSocketHub` | `Asynx[Quiver]` | `^quiver\.` | WebSocket | Pushes Quiver DTO to `/v1/quiver` channels |
