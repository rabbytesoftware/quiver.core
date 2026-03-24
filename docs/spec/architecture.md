# Quiver — Architecture & Implementation Plan

## Overview

This spec defines the project's internal module structure, implementation order, and testing/benchmarking strategy. It is the roadmap for turning the 14 specs into working code.

Related specs: [entities.md](entities.md), [domain.md](domain.md), [commands.md](commands.md), [usecases.md](usecases.md), [wizard.md](wizard.md), [runtime.md](runtime.md), [manifold.md](manifold.md), [vault.md](vault.md), [deptree.md](deptree.md), [netbridge.md](netbridge.md), [http-api.md](http-api.md), [websocket.md](websocket.md), [subscriptions.md](subscriptions.md).

---

## 1. Project Structure

```
internal/
├── domain/                           # Pure domain types — no I/O, no dependencies
├── core/                             # Foundational services — no business logic
│   ├── config/
│   ├── errs/
│   ├── fns/                          # FetchNShare (file ops + HTTP download)
│   │   ├── errors/
│   │   ├── mocks/
│   │   └── strategies/               # Local vs remote
│   ├── metadata/
│   └── watcher/
├── engine/                          # Business logic tools — self-contained, no cross-imports
│   ├── deptree/                      # Dependency graph resolution (DFS topo sort)
│   ├── manifold/                     # Manifest resolution (git → parse → assemble)
│   │   ├── resolver/                 # INTERNAL: shallow git clone → raw bytes
│   │   ├── translator/               # INTERNAL: YAML parse + JSON schema validation
│   │   │   ├── schemas/              # Schema registry + versioned mappers
│   │   │   └── utils/                # Field mapping helpers
│   │   └── assembler/                # INTERNAL: business rules → domain aggregates
│   ├── vault/                        # Namespace home directory + manifest cache
│   ├── netbridge/                    # Port allocation + router forwarding
│   │   ├── models/                   # Shared types (avoids circular imports)
│   │   └── strategies/               # UPnP, NAT-PMP implementations
│   └── wizard/                       # Step execution coordinator
│       └── runtime/                  # INTERNAL: OS process management
│           ├── builder/              # Process config (platform-specific build tags)
│           ├── models/               # Process types and errors
│           ├── output/               # Output capture
│           └── process/              # UnixProcess (darwin+linux), WindowsProcess
├── adapter/                         # Pluggable integrations — bridge to external systems
│   ├── store/                        # Asynx event store backend
│   └── requirements/                 # OS-level system requirements check
├── app/                              # Orchestration layer — composes engines + adapters
│   ├── arrow/
│   │   ├── commands/                 # One file per command (7 total)
│   │   ├── projections/              # One file per handler (3 total)
│   │   └── upcasters/                # One file per upcaster
│   ├── quiver/
│   │   ├── commands/                 # One file per command (3 total)
│   │   ├── projections/              # One file per handler (1 total)
│   │   └── upcasters/
│   └── system/
├── api/                              # HTTP + WebSocket delivery
│   ├── hub.go                        # WebSocketHub interface (version-agnostic)
│   ├── libs/                         # Response envelope, namespace helpers
│   ├── middleware/                   # Logger, request timer, WS upgrade
│   └── v1/
│       ├── endpoints/
│       │   ├── arrows/handlers/
│       │   ├── quivers/handlers/
│       │   ├── health/handlers/
│       │   └── system/handlers/
│       └── ws/                       # WebSocketHub v1 implementation
├── mocks/                            # Cross-module mocks (consumed by app layer tests)
└── internal.go                       # Dependency injection container
```

### 1.1 Layer Distinction

| Layer | Purpose | Examples |
|-------|---------|---------|
| `domain/` | Pure types, no I/O | `Arrow`, `Namespace`, `ArrowState` |
| `core/` | Foundational services shared everywhere | `config`, `watcher`, `fns` |
| `engine/` | Self-contained business logic tools | `deptree`, `manifold`, `vault`, `wizard` |
| `adapter/` | Pluggable integrations with external systems | `store` (Asynx backend), `requirements` (OS) |
| `app/` | Orchestration — the only layer that composes multiple engines | `arrow/`, `quiver/` |
| `api/` | Delivery — maps HTTP/WS to app layer calls | handlers, hub |

**`engine/` vs `adapter/`:** Engines contain algorithms and business logic — they are tools the app layer calls to do work. Adapters contain no business logic — they implement interfaces that plug Quiver into external dependencies (storage engines, OS syscalls). An engine could be tested in isolation with pure in-memory state; an adapter is tested by verifying it correctly delegates to its external system.

### 1.2 Internal Submodule Ownership

Two engines have internal submodules that are owned exclusively by their parent. The app layer never imports them directly.

**Manifold** owns three internal submodules:
- `resolver/` — git fetch logic. Only called by `manifold/manifold.go`.
- `translator/` — YAML parse + schema validation. Only called by `manifold/manifold.go`.
- `assembler/` — business rules → domain aggregates. Only called by `manifold/manifold.go`.

**Wizard** owns one internal submodule:
- `runtime/` — OS process management. Only called by `wizard/wizard.go`.

---

## 2. Module Catalog

### 2.1 Domain (`internal/domain/`)

Pure Go types. No I/O. No imports from other `internal/` packages. Everything else depends on this.

| File | Types | Spec |
|------|-------|------|
| `namespace.go` | `Namespace` + validation | `entities.md` |
| `arrow.go` | `Arrow`, `ArrowManifest`, `Lifecycle` | `domain.md` |
| `quiver.go` | `Quiver`, `QuiverManifest`, `QuiverMedia` | `domain.md` |
| `arrow_state.go` | `ArrowState` enum (absent, installing, ready, running, stopping, uninstalling, removed) | `domain.md` |
| `arrow_runtime.go` | `ArrowRuntime`, `Execution`, `Return`, `StepProgress` | `domain.md` |
| `execution_outcome.go` | `ExecutionOutcome` enum (success, failed, cancelled) | `domain.md` |
| `step.go` | `Step` interface, `BasicStep`, `RunStep`, `FetchStep`, `SignalStep`, `DependenciesStep` | `domain.md` |
| `step_type.go` | `StepType` enum | `domain.md` |
| `step_status.go` | `StepStatus` enum | `domain.md` |
| `variable.go` | `Variable` | `entities.md` |
| `variable_type.go` | `VariableType` enum | `entities.md` |
| `requirement.go` | `Requirement` | `entities.md` |
| `port.go` | `PortDef` | `domain.md` |
| `protocol.go` | `Protocol` enum | `entities.md` |
| `method.go` | `Method` | `domain.md` |
| `os.go` | `OS` | existing |
| `security.go` | `Security` | existing |
| `url.go` | `URL` | existing |
| `forwarding_status.go` | `ForwardingStatus` | existing |

### 2.2 Engines (`internal/engine/`)

Each engine is a self-contained tool. No engine imports another engine. All are called exclusively by the app layer (except translator/resolver/assembler which are internal to manifold, and runtime which is internal to wizard).

#### `deptree/`

| | |
|---|---|
| **Responsibility** | DFS topological sort with 3-color cycle detection |
| **Interface** | `DepTree` — `Resolve(ctx, root Namespace, resolver ResolverFunc) → ([]Namespace, error)` |
| **Types** | `ResolverFunc`, `CycleError` |
| **Errors** | `ErrCyclicDependency` |
| **Spec** | `deptree.md` |

#### `manifold/`

| | |
|---|---|
| **Responsibility** | Git fetch → translate → assemble → `*ArrowManifest` / `*QuiverManifest` |
| **Interface** | `Manifold` — `ResolveArrow(ctx, ns, os) → (*ArrowManifest, error)`, `ResolveQuiver(ctx, ns) → (*QuiverManifest, error)` |
| **Internal submodules** | `resolver/` (git), `translator/` (YAML+schema), `assembler/` (rules) |
| **Types** | `RawArrow`, `RawQuiver` (internal to translator) |
| **Errors** | `ErrNotFound`, `ErrInvalidManifest`, `ErrUnsupportedPlatform`, `ErrFetchFailed` |
| **Spec** | `manifold.md` |

#### `vault/`

| | |
|---|---|
| **Responsibility** | Namespace home directory allocation + manifest JSON cache (TTL-based) |
| **Interface** | `Vault` — `PutArrow`, `GetArrow`, `DeleteArrow`, `PutQuiver`, `GetQuiver`, `DeleteQuiver` |
| **Types** | `VaultEntry`, `QuiverVaultEntry`, `VaultMetadata` |
| **Errors** | `ErrNotCached`, `ErrStale` |
| **Concurrency** | Per-namespace mutexes via `sync.Map` |
| **Spec** | `vault.md` |

#### `netbridge/`

| | |
|---|---|
| **Responsibility** | Dynamic port allocation (preferred port first, fallback 49152–65535) + UPnP/NAT-PMP router forwarding |
| **Interface** | `Netbridge` — `Allocate(ctx, ownerKey, protocol, preferred) → (int, error)`, `DeallocateByOwner(ctx, ownerKey) → error` |
| **Internal aggregate** | `PortAllocation` (Asynx-backed, not exposed via interface) |
| **Submodules** | `models/` (shared types), `strategies/` (UPnP, NAT-PMP) |
| **Errors** | `ErrNoPortAvailable`, `ErrPortOutOfRange`, `ErrAlreadyAllocated` |
| **Spec** | `netbridge.md` |

#### `wizard/`

| | |
|---|---|
| **Responsibility** | Sequential step execution with StepReporter callbacks, per-namespace cancellation |
| **Interface** | `*Wizard` (struct) — `Execute(ctx, req, reporter) → error`, `Cancel(namespace)` |
| **Types** | `ExecutionRequest`, `StepReporter` (callback interface) |
| **Internal submodule** | `runtime/` (process spawn/manage) |
| **Errors** | `ErrUnknownStepType`, `ErrNoProcess`, `ErrExecutionExists` |
| **Spec** | `wizard.md`, `runtime.md` |

#### `wizard/runtime/`

Internal to Wizard. App layer never imports directly.

| | |
|---|---|
| **Responsibility** | OS process spawn, signal, graceful shutdown (SIGTERM → grace period → SIGKILL) |
| **Interface** | `Runtime` — `Get(ctx, command...) → Builder`, `GetByKey(key) → (Process, error)`, `Shutdown(ctx) → error` |
| **Submodules** | `builder/`, `models/`, `output/`, `process/` |
| **Platform** | `UnixProcess` (darwin+linux via `//go:build`), `WindowsProcess` (separate) |
| **Key** | Deterministic UUID v5 from PID + start timestamp |
| **Spec** | `runtime.md` |

### 2.3 Adapters (`internal/adapter/`)

Pluggable implementations. No business logic. They implement interfaces consumed by engines or app layer.

#### `store/`

| | |
|---|---|
| **Responsibility** | Asynx event store backend (persists aggregate event streams) |
| **Consumed by** | App layer — injected into `Asynx[Arrow]`, `Asynx[ArrowRuntime]`, `Asynx[Quiver]`, `Netbridge` |

#### `requirements/`

| | |
|---|---|
| **Responsibility** | OS-level system requirements check (CPU, RAM, disk, platform) |
| **Consumed by** | App layer (informational in v0 — not a blocker for install) |

### 2.4 Application Layer (`internal/app/`)

Orchestration layer. Owns all Asynx instances. Composes engines + adapters. No engine imports another engine here — composition is explicit.

#### `arrow/`

| File | Responsibility |
|------|---------------|
| `service.go` | `ArrowService` struct, constructor, all exported methods |
| `catalog.go` | `Add`, `Update`, `Remove`, `List`, `GetDetail` — CRUD on `Asynx[Arrow]`, cross-aggregate state checks |
| `runtime.go` | `beginExecution`, `executeSync`, `Stop`, `resolveVariables` (6-layer merge), `asynxStepReporter`, `handleExecutionError` |
| `installer.go` | `Install`, `runInstall` — Step 0, DepTree, per-dep loop, rollback |
| `uninstaller.go` | `Uninstall`, `runUninstall` — reverse dep check, root uninstall, orphan cleanup |
| `resolver.go` | `resolveManifest` (Vault cache-first + Manifold fallback), DepTree resolver callback |
| `asynx.go` | Asynx instance setup for `Asynx[Arrow]` + `Asynx[ArrowRuntime]` |

**`commands/`** — one file per command:

| File | Command | Aggregate |
|------|---------|-----------|
| `add.go` | `AddArrow` | `Arrow` |
| `update_manifest.go` | `UpdateArrowManifest` | `Arrow` |
| `remove.go` | `RemoveArrow` | `Arrow` |
| `begin_execution.go` | `BeginExecution` | `ArrowRuntime` |
| `advance_step.go` | `AdvanceStep` | `ArrowRuntime` |
| `end_execution.go` | `EndExecution` | `ArrowRuntime` |
| `mark_stopping.go` | `MarkStopping` | `ArrowRuntime` |

**`projections/`** — one file per handler:

| File | Handler | Trigger |
|------|---------|---------|
| `stop_coordinator.go` | Calls `wizard.Cancel(ns)` | `runtime.MarkStopping` |
| `websocket_runtime.go` | Push ArrowRuntime DTO to hub | `^runtime\.` |
| `websocket_arrow.go` | Push Arrow DTO to hub | `^arrow\.` |

#### `quiver/`

| File | Responsibility |
|------|---------------|
| `service.go` | `QuiverService` struct, constructor |
| `catalog.go` | `Add`, `Update`, `Remove`, `List`, `GetDetail` on `Asynx[Quiver]` |
| `resolver.go` | `resolveManifest` (same cache-first pattern as arrow) |
| `asynx.go` | Asynx instance setup for `Asynx[Quiver]` |

**`commands/`**: `add.go` (`AddQuiver`), `update_manifest.go` (`UpdateQuiverManifest`), `remove.go` (`RemoveQuiver`)

**`projections/`**: `websocket_quiver.go` — push Quiver DTO to hub on `^quiver\.`

### 2.5 API Layer (`internal/api/`)

Delivery only. Calls app layer services. No knowledge of Asynx, commands, or domain internals.

| Module | Responsibility | Spec |
|--------|---------------|------|
| `hub.go` | `WebSocketHub` interface — version-agnostic fan-out | `websocket.md` §7 |
| `v1/ws/` | Hub v1 — domain-to-DTO mapping, connection management, channel routing | `websocket.md` |
| `v1/endpoints/arrows/handlers/` | Arrow REST handlers | `http-api.md` §4 |
| `v1/endpoints/quivers/handlers/` | Quiver REST handlers | `http-api.md` §5 |
| `v1/endpoints/health/handlers/` | Health check | — |
| `v1/endpoints/system/handlers/` | System info | — |
| `middleware/` | Logger, request timer, WS upgrade | — |
| `libs/` | Response envelope, namespace decoding | `http-api.md` §1–2 |

---

## 3. Dependency Graph

### 3.1 Import Rules

1. **`domain/`** imports nothing from `internal/`.
2. **`core/`** imports only `domain/`.
3. **`engine/`** import `domain/` and `core/`. Engine modules never import each other. Wizard imports `core/fns`.
4. **`adapter/`** import `domain/` and `core/`. No business logic. No engine imports.
5. **`app/`** imports `domain/`, `core/`, `engine/`, and `adapter/`. This is the only composition point.
6. **`api/`** imports `domain/` and `app/` service interfaces. Does not import engines or adapters directly.
7. **`internal.go`** (DI container) imports everything to wire them together.

### 3.2 Visual

```
domain/
  ↑ (imported by everyone)
  ├── core/
  │     ↑
  │     ├── engine/       (no cross-imports between engines)
  │     │     ↑
  │     └── adapter/      (no cross-imports between adapters)
  │               ↑
  │               └── app/
  │                     ↑
  │                     └── api/
  │
  └── internal.go  (wires everything)
```

### 3.3 Internal Submodule Isolation

```
engine/manifold/
  ├── manifold.go           ← public surface
  ├── resolver/             ← imported only by manifold.go
  ├── translator/           ← imported only by manifold.go
  └── assembler/            ← imported only by manifold.go

engine/wizard/
  ├── wizard.go             ← public surface
  └── runtime/              ← imported only by wizard.go
```

The app layer sees only `manifold.Manifold` and `*wizard.Wizard`. The internal submodules are implementation details.

---

## 4. Implementation Order

### Phase 0 — Already Done

- **Domain** (partial): `namespace.go`, `variable.go`, `requirement.go`, `protocol.go`, `port.go`, `method.go`, `os.go`, `security.go`, `url.go`, `forwarding_status.go`, `arrow.go`, `quiver.go`
- **Core**: `config/`, `watcher/`, `metadata/`, `fns/`, `errs/`
- **Engines** (partial): `runtime/` (process management, builder, platform-specific), `netbridge/` (partial), `translator/` (partial — will move)
- **API** (skeleton): routes, middleware, handler stubs

### Phase 1 — Foundations

Three independent tracks. Run in parallel.

**Track A — Domain completion:**
- `arrow_state.go`, `arrow_runtime.go`, `execution_outcome.go`, `step.go`, `step_type.go`, `step_status.go`
- Step constructors: `NewRunStep`, `NewFetchStep`, `NewSignalStep`, `NewDependenciesStep`
- Tests: 95%+ coverage. Table-driven for `Namespace.Validate()`, each step constructor, enum validation.

**Track B — DepTree (new):**
- `engine/deptree/deptree.go` — `DepTree` struct, `Resolve()` with DFS topo sort
- `engine/deptree/errors.go` — `ErrCyclicDependency`, `CycleError`
- Tests + benchmarks: linear chain, diamond, wide graph, cycle detection, self-dependency, context cancellation
- Mocks: `internal/mocks/deptree.go`

**Track C — Translator refactor:**
- Change Translator to accept `[]byte` instead of file paths (drop `fns.Read` dependency)
- Move `engine/translator/` → `engine/manifold/translator/`
- Update all tests

### Phase 2 — Engines

Four independent tracks. All depend on Phase 1 (domain types). Track E also depends on Track C.

**Track D — Vault (new):**
- `engine/vault/vault.go` — `Vault` interface + struct, `New()`, Get/Put/Delete, per-namespace mutex, atomic writes
- `engine/vault/models.go` — `VaultEntry`, `QuiverVaultEntry`, `VaultMetadata`
- `engine/vault/errors.go` — `ErrNotCached`, `ErrStale`
- Tests + benchmarks: Get (fresh, stale, not-cached), Put (new, overwrite, atomic), Delete (idempotent, coexisting entries), parallel puts to same namespace
- Mocks: `internal/mocks/vault.go`, `engine/vault/mocks/fs.go`

**Track E — Manifold (new):**
- `engine/manifold/manifold.go` — `Manifold` interface + struct, `ResolveArrow`, `ResolveQuiver`
- `engine/manifold/resolver/` — shallow `go-git` clone into in-memory FS, extract manifest bytes
- `engine/manifold/translator/` — move from Track C, internal
- `engine/manifold/assembler/` — OS override resolution, lifecycle pair validation, step type validation, dependency namespace validation, variable/netbridge uniqueness, timeout parsing
- `engine/manifold/errors.go`
- Tests + benchmarks: full pipeline (bytes → domain), each assembler rule in isolation, error cases, OS override resolution
- Mocks: `internal/mocks/manifold.go`, `engine/manifold/mocks/` (resolver, translator, assembler), `engine/manifold/resolver/mocks/git_client.go`, `engine/manifold/translator/schemas/mocks/registry.go`

**Track F — Wizard refactor:**
- Move `engine/runtime/` → `engine/wizard/runtime/`
- `engine/wizard/wizard.go` — `Wizard` struct, `Execute`, `Cancel`, step dispatch, `executeRunStep`, `executeFetchStep`, `executeSignalStep`
- `engine/wizard/errors.go`
- Consolidate `runtime/process/` darwin+linux into `UnixProcess` (build tags)
- Add `Signal()`, `Done()`, `Key()`, `PID()`, `WithShellWrap()`, `WithGracePeriod()` per `runtime.md`
- Tests + benchmarks: each step type, cancellation mid-step, StepReporter callbacks, signal delivery, process key generation
- Mocks: `internal/mocks/wizard.go`, `engine/wizard/mocks/` (runtime, fns, step_reporter), `engine/wizard/runtime/mocks/process.go`

**Track G — Netbridge completion:**
- Finish Asynx aggregate wiring (commands, events, projection)
- `engine/netbridge/strategies/upnp.go`, `natpmp.go`
- Port allocation: preferred check, fallback range scan, OS bind test
- Tests + benchmarks: allocation (preferred available, preferred taken, range exhaustion), deallocation, concurrent allocation
- Mocks: `internal/mocks/netbridge.go`, `engine/netbridge/mocks/` (read_model_store, stream_store), `engine/netbridge/strategies/mocks/strategy.go`

### Phase 3 — Commands & Projections

Three independent tracks. Depend only on Phase 1A (domain types). **Can run in parallel with Phase 2.**

**Track H — Arrow commands (7 files):**
- One file per command in `app/arrow/commands/`
- Each file: struct + `AggregateID()`, `Validate()`, `EmitEvent()`, `EventName()`, `ShouldSnapshot()`
- Tests: 95%+ coverage. Table-driven for every `Validate()` and `EmitEvent()`.

**Track I — Quiver commands (3 files):**
- Same pattern in `app/quiver/commands/`

**Track J — Projections (4 files):**
- `app/arrow/projections/stop_coordinator.go`
- `app/arrow/projections/websocket_runtime.go`
- `app/arrow/projections/websocket_arrow.go`
- `app/quiver/projections/websocket_quiver.go`
- Tests: mock WebSocketHub and wizard, verify correct delegation

### Phase 4 — App Layer

Sequential. Depends on Phases 2 + 3. Build files in this order — each depends on the previous.

**Arrow** (in order):

1. `asynx.go` — Asynx instance setup, subscription registration
2. `resolver.go` — `resolveManifest`, `buildDepResolver`
3. `catalog.go` — `Add`, `Update`, `Remove`, `List`, `GetDetail`
4. `runtime.go` — `beginExecution`, `executeSync`, `Stop`, `resolveVariables`, `asynxStepReporter`, `handleExecutionError`
5. `installer.go` — `Install`, `runInstall`, `rollbackInstalled`, `updateIndirectDeps`
6. `uninstaller.go` — `Uninstall`, `runUninstall`, `hasDependents`
7. `service.go` — struct, constructor, public method routing

**Quiver** (in order): `asynx.go` → `resolver.go` → `catalog.go` → `service.go`

Use `testhelpers_test.go` per module for shared fixtures. Mock all engine/adapters via `internal/mocks/`.

### Phase 5 — API Layer

Depends on Phase 4.

1. `api/v1/ws/` — WebSocketHub v1: connection management, domain→DTO mapping, channel routing
2. Arrow handlers — HTTP request → `ArrowService`, response envelope, error→status mapping
3. Quiver handlers — HTTP request → `QuiverService`
4. `internal.go` — DI container: construct adapters → engines → app services → API

### 4.1 Dependency Diagram

```
Phase 0 (existing)
  │
  ├── Phase 1A  ──────────────────────────────────────────────┐
  │   (domain)                                                │
  ├── Phase 1B  ──────────────────────────────────────────────┤
  │   (deptree)                                               │
  └── Phase 1C  ──────────────────────────────────────────────┤
      (translator)                                            │
                                                              │
      ┌─────────────────────────────────────────────────────┐ │
      │  Phase 2D   Phase 2E    Phase 2F    Phase 2G        │ │
      │  (vault)    (manifold)  (wizard)    (netbridge)     │◄┤
      │              needs 1C                               │ │
      └──────────────────────────┬──────────────────────────┘ │
                                 │                            │
      ┌──────────────────────────┼──────────────────────────┐ │
      │  Phase 3H   Phase 3I    Phase 3J                    │◄┘
      │  (arrow     (quiver     (projections)               │
      │   commands)  commands)                              │
      └──────────────────────────┬──────────────────────────┘
                                 │
                          Phase 4 (app layer — sequential)
                                 │
                          Phase 5 (API + DI wiring)
```

**Maximum concurrency:** Phases 2 and 3 run fully in parallel (7 simultaneous tracks: D+E+F+G+H+I+J).

---

## 5. Testing Strategy

### 5.1 Frameworks

| Purpose | Tool |
|---------|------|
| Test runner | `go test` |
| Assertions + mocks | `github.com/stretchr/testify` |
| Benchmarks | `testing.B` (standard) |
| Race detection | `go test -race` (already in Makefile) |
| Coverage | `go test -coverprofile` (already in Makefile) |

### 5.2 Coverage Target

**Project-wide minimum: 95%.** Every layer must meet or exceed this.

Coverage is a floor, not a goal. Tests must cover real behavior — valid inputs, invalid inputs, boundary conditions, error paths, state transitions, and concurrency hazards.

**Meaningful tests:**
- Test a specific behavior or invariant: `"namespace with slash is invalid"`
- Cover an error path with a real consequence: `"stale vault entry falls back gracefully to Manifold"`
- Exercise a state machine transition: `"arrow cannot begin execution if already running"`
- Verify concurrency safety: `"parallel Vault puts to the same namespace don't corrupt state"`

**Not meaningful:**
- A test that only exercises the happy path to increment a line counter
- A test that asserts `err == nil` with no edge-case companion
- A test that mirrors the implementation rather than testing the contract

### 5.3 Mock Organization

**Rule: every interface gets a mock. Mocks live at the level where they are consumed.**

#### Level 1 — Cross-module mocks: `internal/mocks/`

Interfaces consumed by the app layer. Shared across `app/arrow/` and `app/quiver/`.

```
internal/mocks/
  vault.go          # mock vault.Vault
  manifold.go       # mock manifold.Manifold
  deptree.go        # mock deptree.DepTree
  netbridge.go      # mock netbridge.Netbridge
  wizard.go         # mock *wizard.Wizard
  asynx.go          # mock asynx.Asynx[T] (if needed)
```

#### Level 2 — Module-internal mocks: `internal/{module}/mocks/`

Interfaces consumed only within a module's own tests.

```
internal/core/fns/mocks/                           # already exists: fs.go, http.go
internal/engine/vault/mocks/
  fs.go                                            # mock filesystem abstraction
internal/engine/manifold/mocks/
  resolver.go                                      # mock resolver for manifold tests
  translator.go                                    # mock translator for manifold tests
  assembler.go                                     # mock assembler for manifold tests
internal/engine/netbridge/mocks/
  read_model_store.go
  stream_store.go
internal/engine/wizard/mocks/
  runtime.go                                       # mock Runtime for wizard tests
  fns.go                                           # mock FNS for wizard tests
  step_reporter.go                                 # mock StepReporter
internal/api/mocks/
  arrow_service.go                                 # mock ArrowService
  quiver_service.go                                # mock QuiverService
```

#### Level 3 — Child-module mocks: `internal/{module}/{child}/mocks/`

Interfaces consumed only within a child module's own tests.

```
internal/engine/manifold/resolver/mocks/
  git_client.go                                    # mock go-git client
internal/engine/manifold/translator/schemas/mocks/
  registry.go                                      # mock schema registry
internal/engine/netbridge/strategies/mocks/
  strategy.go                                      # mock Strategy interface
internal/engine/wizard/runtime/mocks/
  process.go                                       # mock Process interface
internal/engine/wizard/runtime/builder/mocks/
  builder.go                                       # mock Builder
```

### 5.4 Benchmark Strategy

**Rule: synchronous user-facing paths MUST have benchmarks. Async paths (202 responses) SHOULD have benchmarks with less rigor.**

#### Synchronous paths (benchmarks required)

The user blocks waiting for these responses. Performance directly impacts UX.

| Operation | File | Why |
|-----------|------|-----|
| `Add` / `Update` / `Remove` | `app/arrow/catalog.go` | Sync 201/200 — manifest resolution in the request path |
| `List` / `GetDetail` | `app/arrow/catalog.go` | Sync 200 — iterates all aggregates |
| Same for Quiver | `app/quiver/catalog.go` | Same reasons |
| Namespace validation | `domain/namespace.go` | Called on every request |
| Variable resolution | `app/arrow/runtime.go` | 6-layer merge before every execution |
| Vault Get/Put | `engine/vault/vault.go` | Disk I/O on every manifest lookup |
| DepTree Resolve | `engine/deptree/deptree.go` | Graph traversal — O(V+E) |
| Translator parse | `engine/manifold/translator/` | YAML parse + JSON schema validation |
| Assembler | `engine/manifold/assembler/` | OS override resolution + rule evaluation |

Benchmark file naming: `{name}_bench_test.go` colocated with source.

```go
// engine/deptree/deptree_bench_test.go
func BenchmarkResolve_LinearChain10(b *testing.B)  { ... }
func BenchmarkResolve_DiamondDependency(b *testing.B) { ... }
func BenchmarkResolve_Wide50Deps(b *testing.B)     { ... }
```

#### Async paths (benchmarks optional)

These return 202 immediately. Benchmark the hot path but not exhaustively.

| Operation | File |
|-----------|------|
| Install flow | `app/arrow/installer.go` |
| Uninstall flow | `app/arrow/uninstaller.go` |
| Execute / Stop | `app/arrow/runtime.go` |
| Wizard step execution | `engine/wizard/wizard.go` |
| Netbridge allocation | `engine/netbridge/netbridge.go` |

### 5.5 Test Naming Convention

```go
func TestMethodName(t *testing.T)                     // happy path
func TestMethodName_WhenCondition(t *testing.T)       // named scenario
func TestMethodName_ReturnsError(t *testing.T)        // error case
func TestMethodName_WithInvalidInput(t *testing.T)    // validation failure
```

Use table-driven tests for methods with multiple input combinations:

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        input   SomeInput
        wantErr bool
    }{
        {"valid input", validInput(), false},
        {"empty name", emptyNameInput(), true},
        {"slash in name", slashNameInput(), true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) { ... })
    }
}
```

---

## 6. Refactoring Migration Steps

These are mechanical moves that happen before new code is written.

1. **Rename `app/arrows/` → `app/arrow/`**, `app/quivers/` → `app/quiver/` — update all import paths
2. **Split `app/arrow/commands/commands.go`** into 7 files (one per command)
3. **Split `app/arrow/projections/projections.go`** into 3 files (one per handler); same for quiver
4. **Move `engine/translator/`** → `engine/manifold/translator/` — update all imports (Translator is internal to Manifold)
5. **Move `engine/runtime/`** → `engine/wizard/runtime/` — update all imports (Runtime is internal to Wizard)
6. **Move `adapter/requirements/`** from wherever it currently lives in `infrastructure/`
7. **Refactor Translator** to accept `[]byte` instead of file paths (drop `fns.Read` dependency)

---

## 7. Summary

| Decision | Value |
|----------|-------|
| **Logic layer name** | `engine/` |
| **Integration layer name** | `adapter/` |
| **Package naming** | Singular — `arrow/`, `quiver/` |
| **File granularity** | One file per command, projection, upcaster |
| **Manifold internal submodules** | `resolver/`, `translator/`, `assembler/` |
| **Wizard internal submodule** | `runtime/` |
| **No cross-engine imports** | All composition in `app/` |
| **Mock levels** | Cross-module → `internal/mocks/`, module-internal → `{module}/mocks/`, child-internal → `{module}/{child}/mocks/` |
| **Coverage floor** | 95% everywhere, meaningful tests only |
| **Benchmark scope** | Required for sync paths, optional for async |
| **Max parallelism** | 7 concurrent tracks (Phases 2+3 combined) |
| **Critical path** | Domain → Translator → Manifold → App layer → API |
