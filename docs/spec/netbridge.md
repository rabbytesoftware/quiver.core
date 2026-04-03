# Quiver — Netbridge

## Overview

Netbridge is the infrastructure module responsible for **dynamic port allocation and forwarding**. When the app layer needs a port for an Arrow execution, it calls Netbridge with a protocol and preferred port number. Netbridge finds an available port, attempts to forward it through the router, and returns the assigned port number. That's it — the caller gets a number back and assigns it to a variable.

Ports are **ephemeral** — allocated when an Arrow starts executing, released when it stops. An Arrow may receive different ports on each execution. Netbridge does not guarantee port stability across restarts.

Internally, Netbridge owns two concerns:

| Concern | Responsibility |
|---------|----------------|
| **Allocation** | Find an available port, track it in the event store, project it into the read model |
| **Forwarding** | Open the port on the router via UPnP, NAT-PMP, or future strategies |

Both concerns live inside the same module. The app layer sees only the module's top-level interface.

The package lives at `internal/engine/netbridge`.

---

## 1. Interface Contract

The app layer depends on a single interface:

```go
// Netbridge is the interface the app layer depends on.
// Defined in the netbridge package.
type Netbridge interface {
    // Allocate finds an available port matching the given protocol and preferred
    // port, forwards it through the router (best-effort), and returns the
    // assigned port number.
    //
    // If the preferred port is unavailable, Netbridge finds the next available
    // port. If forwarding fails, the port is still allocated locally and
    // returned — the caller can check forwarding status via the read model.
    Allocate(ctx context.Context, ownerKey string, protocol Protocol, preferred int) (int, error)

    // DeallocateByOwner releases all ports allocated to the given owner key.
    // Called when an Arrow's execution ends (natural exit or stop).
    DeallocateByOwner(ctx context.Context, ownerKey string) error
}
```

This is the **only** interface the app layer imports. No router types, no strategy types, no event store details — just protocol and port number in, port number out.

### `Protocol`

Netbridge defines its own protocol type — it does not import from the domain layer.

```go
type Protocol string

const (
    ProtocolTCP    Protocol = "tcp"
    ProtocolUDP    Protocol = "udp"
    ProtocolTCPUDP Protocol = "tcp/udp"
)
```

---

## 2. Builder

Netbridge is constructed via the builder pattern. The caller injects an event store for persistence; the read model store is optional (defaults to in-memory).

```go
nb, err := netbridge.New().
    WithEventStore(asynxStore).           // required
    WithStore(readModelStore).            // optional — defaults to in-memory
    WithEphemeralPortRange(49152, 65535). // optional — defaults from config
    Build(ctx)
```

### Injectable Dependencies

| Dependency | Type | Purpose |
|---|---|---|
| **Event store** | `asynx/models.Store` | Asynx event persistence backend for the `PortAllocation` aggregate |
| **Read model store** *(optional)* | `store.PortStore` (defined below) | Storage for the CQRS projection — defaults to in-memory if not provided |

### Event Store (Asynx Integration)

The event store is the Asynx stream persistence backend (`github.com/char2cs/asynx/models.Store`). The caller provides an implementation — typically backed by SQLite via `internal/adapter/eventstore/sqlite`. Netbridge uses this to persist port allocation events and subscribe to projections.

### `PortStore`

The read model store is a thin CRUD interface for port allocation data. Netbridge owns the projection logic — the store only persists and retrieves.

```go
type PortStore interface {
    Save(alloc PortAllocation) error
    Delete(port int) error
    FindByPort(port int) (*PortAllocation, error)
    FindByOwner(ownerKey string) ([]PortAllocation, error)
    FindByID(id int) (*PortAllocation, error)
    FindAll() ([]PortAllocation, error)
}
```

Note: `DeleteByOwner` is not on the interface. Instead, `DeallocateByOwner` queries the read model via `FindByOwner` and then calls `Delete` for each port individually.

### What the builder does internally

1. Validates that `EventStore` is set — returns `ErrBuildFailed` if missing
2. Initializes the internal Asynx instance with the injected event store
3. Wires up the event projection (Asynx events → `PortStore`)
4. Resolves the read model store: uses injected store if provided, otherwise creates in-memory store
5. Resolves ephemeral port range: uses `WithEphemeralPortRange()` if set, otherwise reads from config defaults
6. Probes the network to detect available forwarding strategies (UPnP, NAT-PMP)
7. Filters to only active strategies and arranges them in fallback chain
8. Returns the ready-to-use `Netbridge` implementation

---

## 3. Internal Aggregate

Netbridge owns a private Asynx aggregate for port allocation tracking.

### `PortAllocation`

```go
type PortAllocation struct {
    Port      int
    Protocol  Protocol
    OwnerKey  string
    Forwarded bool
}
```

| Field | Description |
|-------|-------------|
| `Port` | The allocated port number (aggregate ID) |
| `Protocol` | `tcp`, `udp`, or `tcp/udp` |
| `OwnerKey` | Opaque grouping key provided by the caller — used for bulk deallocation |
| `Forwarded` | Whether router forwarding succeeded for this port |

### Commands & Events

| Command | Validates | Event | Effect |
|---------|-----------|-------|--------|
| `AllocatePort` | Port not already allocated (via read model) | `port.Allocated` | Creates a `PortAllocation` entry |
| `DeallocatePort` | Port exists | `port.Deallocated` | Removes the `PortAllocation` entry |

`DeallocateByOwner` is not a single command — it queries the read model for all ports owned by the key, then dispatches `DeallocatePort` for each.

### Projection

Netbridge subscribes to its own Asynx events and updates the read model store:

| Event | Projection |
|-------|------------|
| `port.Allocated` | `ReadModelStore.Save(allocation)` |
| `port.Deallocated` | `ReadModelStore.Delete(port)` |

This projection logic is baked into Netbridge — it is not configurable or injectable.

---

## 4. Allocation Strategy

When `Allocate` is called:

```
1. If preferred port > 0:
   a. Check read model — is it already allocated?
   b. If not allocated → check OS-level availability (bind test)
   c. If available → use it
2. If preferred unavailable or not specified:
   a. Search for next available port starting from preferred (or range start)
   b. For each candidate: check read model, then OS-level availability
   c. Use the first available
3. Attempt router forwarding via the strategy chain
4. Record allocation in Asynx (with Forwarded = true/false)
5. Project into read model
6. Return the port number
```

### Port Range

Netbridge searches within the ephemeral/dynamic port range: **49152–65535** (IANA recommended, the default).

The port range is configurable in two ways:

1. **Via builder:** `WithEphemeralPortRange(start, end)` sets it explicitly for a `Netbridge` instance
2. **Via config:** `internal/core/config/config.go` reads `ephemeral_port_start` and `ephemeral_port_end` keys; the builder uses these defaults if no explicit range is provided

If a preferred port falls outside the valid range (1–65535), it is rejected with `ErrPortOutOfRange`. The fallback search uses the configured ephemeral range.

### OS-Level Availability Check

Netbridge checks if a port is actually free on the system by attempting a `net.Listen` (TCP) or `net.ListenPacket` (UDP) on the candidate port. The listener is immediately closed. This catches ports bound by other processes that Netbridge doesn't know about.

---

## 5. Forwarding Strategies

Netbridge uses a **strategy pattern** internally for router port forwarding. Strategies are auto-detected at startup and arranged in a fallback chain.

### Strategy Interface

Defined in the `internal/strategies` sub-package. Strategies reference the `Protocol` type from `internal/ports`.

```go
// package internal/strategies

// Strategy defines a port forwarding mechanism.
type Strategy interface {
    // Name returns a human-readable identifier for logging.
    Name() string

    // Available probes the network to determine if this strategy
    // can be used in the current environment.
    Available(ctx context.Context) bool

    // Forward opens the given port on the router.
    Forward(ctx context.Context, port int, protocol ports.Protocol) error

    // Reverse closes the given port on the router.
    Reverse(ctx context.Context, port int, protocol ports.Protocol) error
}
```

### Built-In Strategies

| Strategy | Description | Priority |
|----------|-------------|----------|
| UPnP | Universal Plug and Play — most common home router protocol | 1 (tried first) |
| NAT-PMP | NAT Port Mapping Protocol — Apple/miniupnpd routers | 2 (fallback) |

### Fallback Chain

At startup (`Build()`), Netbridge probes each strategy in priority order via `Available()`. All available strategies are kept in the chain. At forwarding time:

```
1. Try strategy[0].Forward()
2. If error → try strategy[1].Forward()
3. ... continue until success or all exhausted
4. If all fail → allocation still succeeds, Forwarded = false
```

Forwarding failure is **not fatal**. The port is allocated locally regardless. The `Forwarded` field on the allocation records whether external reachability was achieved.

### Adding New Strategies

To add a new forwarding strategy:

1. Create a new file in `internal/infrastructure/netbridge/strategies/` (e.g., `newprotocol.go`)
2. Implement the `strategy` interface
3. Register it in the strategy chain (priority-ordered slice in the builder's probe function)

No changes to the public interface. No changes to the allocation logic. The strategy chain is the only extension point.

---

## 6. Internal Package Structure

```
internal/engine/netbridge/
├── netbridge.go            # Netbridge service implementation
├── builder.go              # Builder pattern constructor
├── allocation.go           # Port search algorithm (preferred → fallback → OS check)
├── errors.go               # Sentinel errors
└── internal/
    ├── commands/
    │   ├── allocate_port.go    # AllocatePort command + validation + event emission
    │   └── deallocate_port.go  # DeallocatePort command + validation
    ├── ports/
    │   ├── port_allocation.go  # PortAllocation struct (aggregate state)
    │   └── protocol.go         # Protocol type and constants
    ├── projections/
    │   └── port_events.go      # Asynx event handler → PortStore projection
    ├── store/
    │   ├── store.go            # PortStore interface definition
    │   ├── memory.go           # In-memory PortStore implementation
    │   └── sqlite.go           # SQLite PortStore implementation
    ├── strategies/
    │   ├── strategy.go         # Strategy interface definition
    │   ├── upnp.go             # UPnP forwarding strategy
    │   └── natpmp.go           # NAT-PMP forwarding strategy
    └── mocks/                  # Test doubles (internal testing only)
```

All shared types live in `internal/ports/` — the `Protocol` and `PortAllocation` types are imported by all sub-packages as needed. The dependency graph is:

```
netbridge          → internal/commands, internal/ports, internal/store, internal/strategies, internal/projections
internal/commands  → internal/ports
internal/store     → internal/ports
internal/strategies → internal/ports
internal/projections → internal/ports, internal/store
```

No circular dependencies.

| File | Responsibility |
|------|----------------|
| `netbridge.go` | `Netbridge` interface + service implementation, `Allocate` / `DeallocateByOwner` methods |
| `builder.go` | Builder pattern constructor, dependency injection, validation, strategy detection, port range resolution |
| `allocation.go` | Port search algorithm: preferred port check, fallback range scan, OS bind test |
| `errors.go` | Package-level sentinel errors |
| `internal/commands/allocate_port.go` | `AllocatePort` command definition, validation, event emission |
| `internal/commands/deallocate_port.go` | `DeallocatePort` command definition, validation |
| `internal/ports/port_allocation.go` | `PortAllocation` aggregate state struct |
| `internal/ports/protocol.go` | `Protocol` type, constants |
| `internal/projections/port_events.go` | Asynx event subscriber, projection logic (events → store) |
| `internal/store/store.go` | `PortStore` interface definition |
| `internal/store/memory.go` | In-memory `PortStore` implementation |
| `internal/store/sqlite.go` | SQLite-backed `PortStore` implementation |
| `internal/strategies/strategy.go` | `Strategy` interface definition |
| `internal/strategies/upnp.go` | UPnP port forwarding strategy |
| `internal/strategies/natpmp.go` | NAT-PMP port forwarding strategy |

---

## 7. Integration with the App Layer

The app layer calls Netbridge during variable assembly — after the manifest is resolved (Manifold) and before variable interpolation.

```
1. App layer reads the netbridge section from the resolved manifest
2. For each port entry:
   a. Call netbridge.Allocate(ctx, ownerKey, protocol, preferred)
   b. If error AND entry is required → abort
   c. If error AND entry is not required → skip
   d. If success → return the allocated port to the app layer
3. App layer maps each port entry name to its allocated port number
   (e.g., GAME_PORT → "27015") and feeds this into the variable
   resolution pipeline (see entities.md § Variable resolution pipeline)
```

Netbridge has no knowledge of variables, merge order, or interpolation — it returns a port number and the app layer decides what to do with it.

On execution end (natural exit or stop):

```
1. App layer calls netbridge.DeallocateByOwner(ctx, ownerKey)
2. Netbridge releases all ports for that owner
3. Router forwarding is reversed for each port
```

The `ownerKey` is set by the app layer — Netbridge does not interpret it.

---

## 8. Error Types

```go
var (
    // ErrNoPortAvailable — all ports in the search range are occupied.
    ErrNoPortAvailable = errors.New("netbridge: no available port found")

    // ErrPortOutOfRange — the requested port is outside the valid range (1–65535).
    ErrPortOutOfRange = errors.New("netbridge: port out of valid range")

    // ErrBuildFailed — the builder is missing required dependencies or Asynx failed to initialize.
    ErrBuildFailed = errors.New("netbridge: failed to build netbridge")
)
```

Errors wrap context via `fmt.Errorf("...: %w", ErrBuildFailed)` so callers can use `errors.Is`.

**Duplicate allocation handling:** When `AllocatePort` is sent to Asynx with a port already in the read model, Asynx's command validation catches it. This is not surfaced as a public error; instead, the allocation logic in `Netbridge` prevents sending duplicate commands in the first place by checking the read model before sending.

**Forwarding failures:** Are **not** exposed as errors to the caller. They are recorded on the `PortAllocation.Forwarded` field. Best-effort reversal during `DeallocateByOwner` silently continues if a strategy's `Reverse()` fails.

---

## 9. Constraints

- **Ephemeral allocations** — ports are per-execution, not per-installation. No persistence across Arrow restarts.
- **Best-effort forwarding** — forwarding failure does not block allocation. The port is always allocated locally.
- **No domain layer knowledge** — Netbridge does not import from `internal/domain`. It defines its own `Protocol` and `PortAllocation` types.
- **No strategy leakage** — strategies are internal. The public interface has no concept of UPnP or NAT-PMP.
- **Caller-provided infrastructure** — the Asynx stream store and read model store are injected via the builder. Netbridge never instantiates its own storage.
- **Projection is internal** — the read model store is a dumb CRUD interface. Netbridge owns the event-to-projection mapping.
- **Extractable** — the module is designed to be extractable as a standalone library. No Quiver-specific types in the public API.

---

## 10. Summary

| Aspect | Decision |
|--------|----------|
| **Scope** | Port allocation + router forwarding |
| **Lifetime** | Ephemeral — allocated per execution, released on stop |
| **Public API** | `Allocate(ctx, ownerKey, protocol, preferred) → (port, error)` and `DeallocateByOwner(ctx, ownerKey)` |
| **State** | Internal Asynx aggregate + CQRS read model |
| **Infrastructure** | Stream store and read model store injected via builder |
| **Forwarding** | Strategy pattern, auto-detected, fallback chain. Failure is non-fatal. |
| **Port range** | Preferred first, then fallback to 49152–65535 |
| **OS check** | `net.Listen` / `net.ListenPacket` bind test |
| **Domain coupling** | None — defines its own types, uses opaque `ownerKey` |

---

## 11. Resolved Questions

All design questions from the specification phase have been resolved:

| # | Question | Resolution |
|---|---|---|
| 1 | Batch port allocation (all-or-nothing) or sequential? | **Resolved:** Sequential allocation. Each `Allocate` call is independent. The app layer can roll back via `DeallocateByOwner(ownerKey)` if any required allocation fails. |
| 2 | Strategy probe once at startup or re-probe periodically? | **Resolved:** Once at `Build()` time. Network topology changes mid-session are rare enough to ignore for now. Re-probing would require periodic background scanning — not currently implemented. |
| 3 | Reverse router forwarding on deallocation? | **Resolved:** Yes — `DeallocateByOwner` calls `Reverse()` on each strategy for each port (best-effort, non-blocking). Then sends `DeallocatePort` commands to Asynx. Full cleanup. |
