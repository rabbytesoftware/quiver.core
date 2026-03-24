# Quiver — HTTP API

## Overview

The HTTP API is the external interface to Quiver.core. It is an infrastructure module — it accepts HTTP requests, extracts parameters, calls into the use case layer, and returns responses. It has no knowledge of domain commands, Asynx, or execution internals.

Base path: `/v1`

Related specs: [commands.md](commands.md) (underlying state transitions), [subscriptions.md](subscriptions.md) (WebSocket real-time feed), [domain.md](domain.md) (aggregate definitions).

---

## 1. Namespace Encoding

Namespaces contain `/` characters that conflict with URL path parsing. To avoid ambiguity, **namespaces are percent-encoded into a single path segment**.

| Raw Namespace | Encoded Path Segment |
|---|---|
| `github.com/valve/steamcmd` | `github.com%2Fvalve%2Fsteamcmd` |
| `github.com/char2cs/gaming.quiver/cs2` | `github.com%2Fchar2cs%2Fgaming.quiver%2Fcs2` |

The server decodes the path parameter before using it as an aggregate ID. The `{namespace}` placeholder in all endpoint definitions below refers to the **encoded** form.

---

## 2. Response Envelope

All responses use a consistent JSON envelope.

### Mutation response

Returned by `POST`, `PATCH`, `DELETE` on resources, and `POST` on method invocations.

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/valve/steamcmd"
}
```

### Query response

Returned by `GET` endpoints. `data` is an array for list endpoints, an object for detail endpoints.

```json
{
  "success": true,
  "error": null,
  "data": { }
}
```

### Error response

Returned on failure for any verb.

```json
{
  "success": false,
  "error": "arrow already exists",
  "namespace": "github.com/valve/steamcmd"
}
```

---

## 3. Error Catalog

The HTTP layer maps use case errors to status codes. The use case layer provides the error message string.

| HTTP Status | Condition | Example `error` |
|---|---|---|
| `400 Bad Request` | Invalid namespace format, malformed request body | `"invalid namespace format"` |
| `404 Not Found` | Arrow or Quiver does not exist | `"arrow not found"` |
| `409 Conflict` | Arrow/Quiver already exists (on Add), already removed (on Remove), or execution already in progress | `"arrow already exists"`, `"arrow already removed"` |
| `422 Unprocessable Entity` | State violation — operation not valid in current lifecycle state, resource has been removed (on Update), or reverse dependency check failed (on Uninstall) | `"arrow must be in state 'ready' to execute"`, `"arrow has been removed"`, `"other arrows depend on this arrow"` |
| `502 Bad Gateway` | Manifold fetch failure (git remote unreachable, manifest parse error) | `"failed to resolve namespace: fetch failed"` |
| `500 Internal Server Error` | Unexpected server error | `"internal error"` |

---

## 4. Arrow Endpoints

### 4.1 Add Arrow — `POST /v1/arrow/{namespace}`

Resolves the namespace via Manifold (git fetch + manifest parse) and stores the Arrow. Triggers `arrow.Add`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `201 Created`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/valve/steamcmd"
}
```

**Errors:** `400` (invalid namespace), `409` (already exists), `502` (fetch failed)

---

### 4.2 Update Arrow Manifest — `PATCH /v1/arrow/{namespace}`

Re-fetches the manifest from the upstream git remote and updates the stored Arrow. Triggers `arrow.UpdateManifest`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/valve/steamcmd"
}
```

**Errors:** `404` (not found), `422` (arrow has been removed), `502` (fetch failed)

---

### 4.3 Remove Arrow — `DELETE /v1/arrow/{namespace}`

Removes the Arrow from the catalog. The use case layer rejects this if the Arrow has not been uninstalled. Triggers `arrow.Remove`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/valve/steamcmd"
}
```

**Errors:** `404` (not found), `409` (arrow already removed), `422` (not uninstalled — runtime still active)

---

### 4.4 List Arrows — `GET /v1/arrow`

Returns all stored Arrows with their current lifecycle state.

**Behavior:** Synchronous
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "data": [
    {
      "namespace": "github.com/valve/steamcmd",
      "name": "SteamCMD",
      "version": "0.0.1",
      "description": "Valve's command-line Steam client",
      "state": "ready",
      "tags": ["utility", "valve"],
      "removed": false
    },
    {
      "namespace": "github.com/char2cs/gaming.quiver/cs2",
      "name": "Counter-Strike 2 Dedicated Server",
      "version": "0.0.1",
      "description": "A basic CS2 SRCDS dedicated server",
      "state": "running",
      "tags": ["game-server", "valve", "fps"],
      "removed": false
    }
  ]
}
```

The `state` field is derived from `ArrowRuntime`. If `ArrowRuntime` is nil (Arrow has never been installed), `state` is `null`. When present, it is one of: `absent`, `installing`, `ready`, `running`, `stopping`, `uninstalling`, `removed`.

---

### 4.5 Get Arrow Detail — `GET /v1/arrow/{namespace}`

Returns the full Arrow manifest and runtime state.

**Behavior:** Synchronous
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "data": {
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
    "indirect_dependencies": ["github.com/valve/steam-runtime"],
    "variables": [
      {
        "name": "SERVER_HOSTNAME",
        "type": "string",
        "default": "CS2 Server hosted with Quiver",
        "description": "Server display name"
      },
      {
        "name": "MAX_PLAYERS",
        "type": "number",
        "default": 12,
        "min": 2,
        "max": 64,
        "description": "Maximum concurrent players"
      },
      {
        "name": "SERVER_PASSWORD",
        "type": "string",
        "default": "",
        "sensitive": true,
        "description": "Server access password"
      },
      {
        "name": "DEFAULT_MAP",
        "type": "select",
        "default": "de_dust2",
        "values": ["de_dust2", "de_mirage", "de_inferno", "de_anubis"],
        "description": "Default map to load"
      }
    ],
    "netbridge": [
      {
        "name": "GAME_PORT",
        "protocol": "tcp/udp",
        "default": 27015,
        "required": true
      },
      {
        "name": "RCON_PORT",
        "protocol": "tcp",
        "default": 27015,
        "required": false
      }
    ],
    "methods": ["update", "validate", "change-map", "backup"],
    "removed": false,
    "state": "running",
    "execution": {
      "method": "_execute",
      "steps": [
        {
          "index": 0,
          "title": "Starting CS2 server",
          "status": "running",
          "error": null
        }
      ],
      "variables": {
        "SERVER_HOSTNAME": "My CS2 Server",
        "MAX_PLAYERS": "32",
        "SERVER_PASSWORD": "",
        "DEFAULT_MAP": "de_dust2",
        "GAME_PORT": "27015",
        "RCON_PORT": "27015"
      }
    },
    "last_return": {
      "method": "_install",
      "outcome": "success",
      "steps": [
        {
          "index": 0,
          "title": "Resolving dependencies",
          "status": "completed",
          "error": null
        },
        {
          "index": 1,
          "title": "Installing CS2 via SteamCMD",
          "status": "completed",
          "error": null
        },
        {
          "index": 2,
          "title": "Configuring server",
          "status": "completed",
          "error": null
        }
      ],
      "variables": {
        "SERVER_HOSTNAME": "My CS2 Server",
        "MAX_PLAYERS": "32",
        "SERVER_PASSWORD": "",
        "DEFAULT_MAP": "de_dust2",
        "GAME_PORT": "27015",
        "RCON_PORT": "27015"
      }
    }
  }
}
```

**Field notes:**

- `methods` is a string array of custom method names (not the full step definitions — those are manifest internals).
- `state` is `null` when `ArrowRuntime` is nil (Arrow has never been installed), otherwise one of: `absent`, `installing`, `ready`, `running`, `stopping`, `uninstalling`, `removed`. `absent` means install was attempted but failed or was cancelled.
- `execution` is `null` when no execution is in progress.
- `last_return` is `null` if no execution has ever completed. Records the outcome, final step statuses, and variables of the most recent completed execution.
- `indirect_dependencies` is `null` before the Arrow has been installed. After a successful install, it contains all transitive dependencies resolved by DepTree that are not direct dependencies. Sourced from the Vault entry (see `vault.md` §4.5). The use case layer queries Vault for the stored indirect dependencies when assembling the detail response.

**Errors:** `404` (not found)

---

### 4.6 Invoke Method — `POST /v1/arrow/{namespace}/{method}`

Executes a lifecycle method or custom method on an Arrow. The `{method}` path segment is the method name: `_install`, `_execute`, `_stop`, `_uninstall`, or any developer-defined method name.

**Behavior:** Asynchronous — returns immediately after the use case layer accepts the command. Execution progress is delivered via WebSocket ([subscriptions.md](subscriptions.md)).

**Request body:** Flat JSON key-value pairs for variables. Optional — omit or send `{}` if no variables are needed.

```json
{
  "SERVER_HOSTNAME": "My CS2 Server",
  "MAX_PLAYERS": "32"
}
```

**Success:** `202 Accepted`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/char2cs/gaming.quiver/cs2"
}
```

**Errors:** `400` (invalid namespace, missing required variables), `404` (arrow not found), `409` (execution already in progress), `422` (state violation — e.g., `_execute` when not `ready`, custom method not available in current state)

**Notes:**

- `_install` triggers the full install flow. The execution's step list begins with a synthetic **Step 0** of type `dependencies` (title: "Resolving dependencies") representing DepTree dependency resolution, followed by the manifest's install steps re-indexed from 1. The use case layer runs **DepTree** to resolve the complete dependency graph (see `deptree.md`). If resolution fails (cycle detected, manifest fetch failure), Step 0 is marked `failed` with the error in `StepProgress.Error`, the install transitions to `absent`, and the error is reported via WebSocket. If resolution succeeds, dependencies are installed in topological order before the root arrow. If a dependency install fails mid-chain, already-installed dependencies are **rolled back** (uninstalled in reverse order, best-effort). After installation completes, the Vault entry is updated with `indirect_dependencies` (see `vault.md` §4.5).
- `_uninstall` triggers the full uninstall flow. The use case layer first performs a **reverse dependency check** — if any other installed arrow depends on this arrow (directly or indirectly), the request is rejected with `422` and `"other arrows depend on this arrow"`. After the root arrow's uninstall steps complete, the use case layer performs **orphaned dependency cleanup** — dependencies (direct + indirect) that are not referenced by any other installed arrow are uninstalled in reverse topological order. Each dependency's uninstall is a full Asynx command sequence visible via WebSocket. See `deptree.md` §Uninstall Flow.
- `_stop` sends `runtime.MarkStopping` to the use case layer. All other methods send `runtime.Begin`. Calling `_stop` when the Arrow is not in `running` state returns `422` with `"arrow is not running"`. The full stop coordination flow (cancel `_execute`, run stop lifecycle steps) is documented in [wizard.md § Stop Flow](wizard.md#stop-flow--full-sequence).
- The use case layer resolves variables (merging request body with stored defaults and built-in variables) before dispatching execution.
- If a required variable is missing and has no default, the use case layer rejects the request with `400`.

---

## 5. Quiver Endpoints

### 5.1 Add Quiver — `POST /v1/quiver/{namespace}`

Resolves the namespace via Manifold and stores the Quiver. Triggers `quiver.Add`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `201 Created`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/char2cs/gaming.quiver"
}
```

**Errors:** `400` (invalid namespace), `409` (already exists), `502` (fetch failed)

---

### 5.2 Update Quiver Manifest — `PATCH /v1/quiver/{namespace}`

Re-fetches the manifest from the upstream git remote. Triggers `quiver.UpdateManifest`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/char2cs/gaming.quiver"
}
```

**Errors:** `404` (not found), `422` (quiver has been removed), `502` (fetch failed)

---

### 5.3 Remove Quiver — `DELETE /v1/quiver/{namespace}`

Removes the Quiver from the catalog. Triggers `quiver.Remove`.

**Behavior:** Synchronous
**Request body:** None
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "namespace": "github.com/char2cs/gaming.quiver"
}
```

**Errors:** `404` (not found), `409` (quiver already removed)

---

### 5.4 List Quivers — `GET /v1/quiver`

Returns all stored Quivers.

**Behavior:** Synchronous
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "data": [
    {
      "namespace": "github.com/char2cs/gaming.quiver",
      "name": "Gaming Quiver",
      "description": "Game servers and utilities curated by char2cs",
      "tags": ["gaming", "servers"],
      "arrow_count": 4,
      "removed": false
    }
  ]
}
```

`arrow_count` is the total number of Arrows listed in the Quiver manifest (both local and external).

---

### 5.5 Get Quiver Detail — `GET /v1/quiver/{namespace}`

Returns the full Quiver manifest with its Arrow catalog.

**Behavior:** Synchronous
**Success:** `200 OK`

```json
{
  "success": true,
  "error": null,
  "data": {
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
}
```

**Field notes:**

- `arrows` lists fully-qualified namespaces. Local AUIDs (e.g., `cs2`) are expanded to full namespaces (e.g., `github.com/char2cs/gaming.quiver/cs2`).

**Errors:** `404` (not found)

---

## 6. Async vs Sync Summary

| Endpoint | Behavior | Success Status |
|---|---|---|
| `POST /v1/arrow/{namespace}` | Sync | `201 Created` |
| `PATCH /v1/arrow/{namespace}` | Sync | `200 OK` |
| `DELETE /v1/arrow/{namespace}` | Sync | `200 OK` |
| `GET /v1/arrow` | Sync | `200 OK` |
| `GET /v1/arrow/{namespace}` | Sync | `200 OK` |
| `POST /v1/arrow/{namespace}/{method}` | **Async** | `202 Accepted` |
| `POST /v1/quiver/{namespace}` | Sync | `201 Created` |
| `PATCH /v1/quiver/{namespace}` | Sync | `200 OK` |
| `DELETE /v1/quiver/{namespace}` | Sync | `200 OK` |
| `GET /v1/quiver` | Sync | `200 OK` |
| `GET /v1/quiver/{namespace}` | Sync | `200 OK` |

Async endpoints return immediately after the use case layer accepts the command. The client observes execution progress by connecting to the WebSocket feed ([subscriptions.md](subscriptions.md)).

---

## 7. Open Questions

| # | Question | Default if unresolved |
|---|---|---|
| 1 | Should `GET /v1/arrow` support query parameters for filtering (by state, tag, quiver)? | No filtering in v0 — return all. |
| 2 | Should `GET /v1/arrow/{namespace}` include full method definitions (steps, `available_in`) or just method names? | Just names — full definitions are manifest internals. |
| 3 | Should there be pagination for list endpoints? | No pagination in v0 — return all. |
| 4 | Should there be a dedicated `GET /v1/arrow/{namespace}/runtime` endpoint for just the runtime state? | No — runtime is included in the detail response. |
