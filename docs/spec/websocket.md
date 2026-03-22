# Quiver — WebSocket Protocol

## Overview

The WebSocket layer is a one-way notification pipe. When Asynx events fire, the use case layer passes the domain aggregate to a version-agnostic hub. The hub fans out to each API version's WebSocket module, which maps the domain type to its versioned DTO and pushes it to connected clients. The WS layer has no business logic — it only maps and delivers.

Clients connect to resource-scoped endpoints. The endpoint URL is the subscription — no subscribe/unsubscribe protocol, no room joins. Connect and receive.

Base path: `/v1`

Related specs: [commands.md](commands.md) (events that trigger pushes), [domain.md](domain.md) (aggregate definitions), [http-api.md](http-api.md) (REST endpoints and DTO shapes), [subscriptions.md](subscriptions.md) (Asynx handler wiring).

---

## 1. Design Principles

- **URL-as-subscription** — no room joins, no subscribe/unsubscribe messages. Connect to an endpoint, receive pushes.
- **Aggregate push model** — every push is the full versioned DTO (same shapes as REST responses), not a custom event message. The aggregate state tells the client what happened.
- **No initial snapshot** — client fetches current state via REST, then connects WS for incremental updates.
- **One-way pipe** — server to client only. Client sends nothing except protocol-level pong frames.
- **Fire and forget** — no delivery guarantees, no message IDs, no acknowledgement.
- **No auth in v0** — all endpoints are open.
- **Protocol-level heartbeat** — WebSocket ping/pong (RFC 6455 opcodes 0x9/0xA), not application-level.
- **Versioned** — WS and REST live under the same `/v1/` path and share the same DTO definitions. Future API versions can reshape DTOs independently.

---

## 2. Namespace Encoding

Namespaces contain `/` characters that conflict with URL path parsing. To avoid ambiguity, **namespaces are percent-encoded into a single path segment** — same rule as the REST API ([http-api.md §1](http-api.md#1-namespace-encoding)).

| Raw Namespace | Encoded Path Segment |
|---|---|
| `github.com/valve/steamcmd` | `github.com%2Fvalve%2Fsteamcmd` |
| `github.com/char2cs/gaming.quiver/cs2` | `github.com%2Fchar2cs%2Fgaming.quiver%2Fcs2` |

The `{namespace}` placeholder in all endpoint definitions below refers to the **encoded** form.

---

## 3. Endpoints

| Endpoint | DTO Pushed | Scope |
|---|---|---|
| `ws://host/v1/arrow` | Arrow DTO | All arrows — catalog changes |
| `ws://host/v1/arrow/{namespace}` | Arrow DTO | Single arrow — catalog changes |
| `ws://host/v1/arrow.runtime` | ArrowRuntime DTO | All arrows — execution events |
| `ws://host/v1/arrow.runtime/{namespace}` | ArrowRuntime DTO | Single arrow — execution events |
| `ws://host/v1/quiver` | Quiver DTO | All quivers — catalog changes |
| `ws://host/v1/quiver/{namespace}` | Quiver DTO | Single quiver — catalog changes |

Each aggregate has one DTO shape — the same for both global and namespace channels. The global channel carries events for all aggregates of that type; the namespace channel filters to one. Arrow and Quiver DTOs are purely catalog — no runtime state. Runtime state lives exclusively on the ArrowRuntime channels.

---

### 3.1 Arrow Catalog — `ws://host/v1/arrow` and `ws://host/v1/arrow/{namespace}`

Pushes the Arrow DTO on every catalog change. The global endpoint carries events for all arrows; the namespace endpoint filters to one.

**Triggers:** `arrow.Add`, `arrow.UpdateManifest`, `arrow.Remove`

```json
{
  "namespace": "github.com/char2cs/gaming.quiver/cs2",
  "name": "Counter-Strike 2 Dedicated Server",
  "description": "A basic CS2 SRCDS dedicated server",
  "version": "0.0.1",
  "license": "MIT",
  "url": "https://developer.valvesoftware.com/wiki/Counter-Strike_2",
  "maintainers": ["char2cs"],
  "credits": ["Valve Software"],
  "tags": ["game-server", "valve", "fps"],
  "requirements": {
    "cpu_cores": 2,
    "memory_gb": 4,
    "disk_gb": 30,
    "os": ["linux", "windows"]
  },
  "dependencies": ["github.com/valve/steamcmd"],
  "variables": [
    {
      "name": "SERVER_HOSTNAME",
      "type": "string",
      "default": "CS2 Server hosted with Quiver",
      "description": "Server display name"
    }
  ],
  "netbridge": [
    {
      "name": "GAME_PORT",
      "protocol": "tcp/udp",
      "default": 27015,
      "required": true
    }
  ],
  "methods": ["update", "validate", "change-map", "backup"],
  "removed": false
}
```

This is the Arrow aggregate — purely catalog. No `state`, `current_execution`, or `resolved_variables` fields. Runtime state lives on the ArrowRuntime channels. The `removed` field is `true` when the Arrow has been tombstoned via `arrow.Remove`.

---

### 3.2 ArrowRuntime — `ws://host/v1/arrow.runtime` and `ws://host/v1/arrow.runtime/{namespace}`

Pushes the ArrowRuntime DTO on every execution event. The global endpoint carries events for all arrows; the namespace endpoint filters to one.

**Triggers:** `runtime.Begin`, `runtime.Advance`, `runtime.RecordPID`, `runtime.MarkStopping`, `runtime.End`

```json
{
  "namespace": "github.com/char2cs/gaming.quiver/cs2",
  "state": "running",
  "current_execution": {
    "method": "_execute",
    "pid": 12345,
    "steps": [
      {
        "index": 0,
        "title": "Starting CS2 server",
        "status": "running",
        "error": null
      }
    ]
  },
  "resolved_variables": {
    "SERVER_HOSTNAME": "My CS2 Server",
    "MAX_PLAYERS": "32"
  }
}
```

`runtime.Advance` is high-frequency — fires twice per step (pending→running, running→completed/failed). This is the endpoint clients use for live execution progress.

---

### 3.3 Quiver Catalog — `ws://host/v1/quiver` and `ws://host/v1/quiver/{namespace}`

Pushes the Quiver DTO on every catalog change. The global endpoint carries events for all quivers; the namespace endpoint filters to one.

**Triggers:** `quiver.Add`, `quiver.UpdateManifest`, `quiver.Remove`

```json
{
  "namespace": "github.com/char2cs/gaming.quiver",
  "name": "Gaming Quiver",
  "description": "Game servers and utilities curated by char2cs",
  "url": "https://gaming.quiver.ar",
  "maintainers": ["char2cs"],
  "tags": ["gaming", "servers"],
  "media": {
    "icon": "https://example.com/icon.png",
    "banner": "https://example.com/banner.png"
  },
  "arrows": [
    "github.com/char2cs/gaming.quiver/cs2",
    "github.com/char2cs/gaming.quiver/minecraft",
    "github.com/valve/steamcmd"
  ],
  "removed": false
}
```

This is the Quiver aggregate — purely catalog. The `removed` field is `true` when the Quiver has been tombstoned via `quiver.Remove`.

---

## 4. DTO Definitions

Each aggregate has one DTO shape used on both its global and namespace channels. These are the aggregate representations — no response envelope wrapping.

### 4.1 Arrow DTO

The Arrow aggregate — purely catalog. No runtime state.

| Field | Type | Notes |
|---|---|---|
| `namespace` | `string` | |
| `name` | `string` | |
| `description` | `string` | |
| `version` | `string` | |
| `license` | `string` | |
| `url` | `string` | |
| `maintainers` | `string[]` | |
| `credits` | `string[]` | |
| `tags` | `string[]` | |
| `requirements` | `object` | `{ cpu_cores, memory_gb, disk_gb, os: string[] }` |
| `dependencies` | `string[]` | Full namespaces |
| `variables` | `Variable[]` | `{ name, type, default, description, min?, max?, sensitive?, values? }` |
| `netbridge` | `PortDef[]` | `{ name, protocol, default, required }` |
| `methods` | `string[]` | Custom method names |
| `removed` | `bool` | `true` when tombstoned via `arrow.Remove` |

### 4.2 ArrowRuntime DTO

The ArrowRuntime aggregate — execution state.

| Field | Type | Notes |
|---|---|---|
| `namespace` | `string` | Always present — clients on global channel use it for routing |
| `state` | `string` | `installing`, `ready`, `running`, `stopping`, `removed` |
| `current_execution` | `object \| null` | `null` when no execution is in progress |
| `current_execution.method` | `string` | `_install`, `_execute`, `_stop`, `_uninstall`, or custom method name |
| `current_execution.pid` | `int \| null` | `null` until `runtime.RecordPID` fires |
| `current_execution.steps` | `StepProgress[]` | |
| `current_execution.steps[].index` | `int` | |
| `current_execution.steps[].title` | `string` | |
| `current_execution.steps[].status` | `string` | `pending`, `running`, `completed`, `failed` |
| `current_execution.steps[].error` | `string \| null` | |
| `resolved_variables` | `object` | Key-value map. Persists between executions. |

### 4.3 Quiver DTO

The Quiver aggregate — purely catalog.

| Field | Type | Notes |
|---|---|---|
| `namespace` | `string` | |
| `name` | `string` | |
| `description` | `string` | |
| `url` | `string` | |
| `maintainers` | `string[]` | |
| `tags` | `string[]` | |
| `media` | `object` | `{ icon: string, banner: string }` |
| `arrows` | `string[]` | Fully-qualified namespaces |
| `removed` | `bool` | `true` when tombstoned via `quiver.Remove` |

---

## 5. Event-to-Push Mapping

### Arrow catalog feed — `Asynx[Arrow]`, pattern `^arrow\.`

| Event | `/v1/arrow` | `/v1/arrow/{ns}` |
|---|---|---|
| `arrow.Add` | Arrow DTO | Arrow DTO |
| `arrow.UpdateManifest` | Arrow DTO | Arrow DTO |
| `arrow.Remove` | Arrow DTO | Arrow DTO |

### ArrowRuntime feed — `Asynx[ArrowRuntime]`, pattern `^runtime\.`

| Event | `/v1/arrow.runtime` | `/v1/arrow.runtime/{ns}` |
|---|---|---|
| `runtime.Begin` | ArrowRuntime DTO | ArrowRuntime DTO |
| `runtime.Advance` | ArrowRuntime DTO | ArrowRuntime DTO |
| `runtime.RecordPID` | ArrowRuntime DTO | ArrowRuntime DTO |
| `runtime.MarkStopping` | ArrowRuntime DTO | ArrowRuntime DTO |
| `runtime.End` | ArrowRuntime DTO | ArrowRuntime DTO |

Runtime events push **only** on ArrowRuntime channels. They do not push on Arrow catalog channels.

### Quiver catalog feed — `Asynx[Quiver]`, pattern `^quiver\.`

| Event | `/v1/quiver` | `/v1/quiver/{ns}` |
|---|---|---|
| `quiver.Add` | Quiver DTO | Quiver DTO |
| `quiver.UpdateManifest` | Quiver DTO | Quiver DTO |
| `quiver.Remove` | Quiver DTO | Quiver DTO |

---

## 6. Connection Lifecycle

### Upgrade

Standard HTTP upgrade at the endpoint URL. Server responds with `101 Switching Protocols`. On failure:

| Condition | Response |
|---|---|
| Invalid path | `400 Bad Request` |
| Namespace not found | `404 Not Found` |
| Upgrade failure | `400 Bad Request` |

### Steady state

Server pushes JSON text frames. Client sends nothing — server ignores any client frames except pong.

### Heartbeat

Server sends WebSocket ping frames every 30 seconds. Client must respond with pong (most WS libraries handle this automatically). Server closes the connection if no pong is received within 60 seconds.

### Disconnect

Either side may close. Server sends a close frame with status `1000` (normal closure) on shutdown.

### Reconnection

No reconnection protocol. On disconnect, the client should:

1. Fetch current state via REST (`GET` endpoints)
2. Re-establish the WebSocket connection

This ensures the client never misses state changes that occurred during the disconnect window.

---

## 7. Hub Architecture

### Interface

The use case layer sees a single interface — three methods, one per aggregate. The hub extracts the namespace from the aggregate and handles all internal routing (global channels, namespace channels, version fan-out).

```go
type WebSocketHub interface {
    BroadcastArrow(arrow Arrow)
    BroadcastArrowRuntime(runtime ArrowRuntime)
    BroadcastQuiver(quiver Quiver)
}
```

The interface is defined in the use case layer (dependency inversion). The caller passes `event.Aggregate` directly — it has no knowledge of channels, namespaces, DTOs, or API versions.

### Implementation layers

The hub implementation is **version-agnostic** and lives at the API layer root, not inside any version folder. It coordinates between version-specific WebSocket modules.

```
internal/api/
├── hub.go                         # WebSocketHub interface + version-agnostic implementation
├── v1/
│   ├── ws/
│   │   ├── handler.go             # v1: domain→DTO mapping, connection management, channel routing
│   │   └── ...
│   └── endpoints/                 # existing REST handlers
├── v2/                            # future
│   └── ws/
│       └── handler.go             # v2: domain→DTO mapping, own connections
```

**Flow:**

1. Use case layer calls `hub.BroadcastArrow(arrow)`
2. Hub iterates over registered version modules (v1, v2, ...)
3. Each version module maps `Arrow` → versioned DTO, then pushes to:
   - The global channel (`/vN/arrow`) — all connected clients
   - The namespace channel (`/vN/arrow/{namespace}`) — clients watching that specific arrow
4. The version module owns its connections, DTO mapping, and channel routing

The version-agnostic hub is a thin fan-out coordinator. Each version's WS module is self-contained — it manages its own connections, maps domain types to its own DTOs, and knows its own endpoint structure. This is the same pattern as the HTTP handlers: the API layer maps domain to wire format, the use case layer only speaks domain.

---

## 8. Open Questions

| # | Question | Default if unresolved |
|---|---|---|
| 1 | Should `runtime.Advance` pushes on the global channel be throttled/debounced to avoid flooding clients watching all arrows? | No throttling in v0. |
| 2 | Should there be a `ws://host/v1/quiver/{namespace}/arrows` endpoint that pushes Arrow events scoped to a Quiver's arrow list? | No in v0 — client connects to individual Arrow channels. |
| 3 | Maximum connections per client? | No limit in v0. |
