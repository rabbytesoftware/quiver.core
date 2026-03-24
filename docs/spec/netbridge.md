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

The package lives at `internal/infrastructure/netbridge`.

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

Netbridge is constructed via the builder pattern. The caller injects two infrastructure dependencies — the module owns everything else.

```go
nb, err := netbridge.NewBuilder().
    WithStreamStore(asynxStore).
    WithReadModel(readModelStore).
    Build()
```

### Injectable Dependencies

| Dependency | Interface | Purpose |
|------------|-----------|---------|
| **Stream store** | Asynx stream storage interface | Event persistence for the internal `PortAllocation` aggregate |
| **Read model store** | `ReadModelStore` (defined below) | Storage backend for the CQRS projection |

### `StreamStore`

The stream store is the Asynx event persistence backend. Netbridge defines a minimal interface for what it needs — the caller provides an implementation backed by whatever storage they choose.

```go
type StreamStore interface {
    Send(ctx context.Context, cmd interface{}) error
}
```

> This interface will align with the real Asynx library's stream store contract once Asynx is integrated. For now it serves as the dependency boundary.

### `ReadModelStore`

The read model store is a thin CRUD interface. Netbridge owns the projection logic — the store only stores.

```go
type ReadModelStore interface {
    Save(allocation PortAllocation) error
    Delete(port int) error
    DeleteByOwner(ownerKey string) error
    FindByPort(port int) (*PortAllocation, error)
    FindByOwner(ownerKey string) ([]PortAllocation, error)
}
```

### What the builder does internally

1. Validates that both dependencies are provided
2. Initializes the internal Asynx instance with the injected stream store
3. Wires up the event projection (Asynx events → read model store)
4. Probes the network to detect available forwarding strategies (auto-detection)
5. Builds the strategy fallback chain
6. Returns the ready-to-use `Netbridge` implementation

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

Netbridge searches within the ephemeral/dynamic port range: **49152–65535** (IANA recommended). If a preferred port falls outside this range, it is still tried first — the range is only used for fallback search.

### OS-Level Availability Check

Netbridge checks if a port is actually free on the system by attempting a `net.Listen` (TCP) or `net.ListenPacket` (UDP) on the candidate port. The listener is immediately closed. This catches ports bound by other processes that Netbridge doesn't know about.

---

## 5. Forwarding Strategies

Netbridge uses a **strategy pattern** internally for router port forwarding. Strategies are auto-detected at startup and arranged in a fallback chain.

### Strategy Interface

Defined in the `strategies` sub-package. Both `strategies` and `netbridge` import shared types from `netbridge/models`, avoiding circular imports.

```go
// package strategies

// Strategy defines a port forwarding mechanism.
type Strategy interface {
    // Name returns a human-readable identifier for logging.
    Name() string

    // Available probes the network to determine if this strategy
    // can be used in the current environment.
    Available(ctx context.Context) bool

    // Forward opens the given port on the router.
    Forward(ctx context.Context, port int, protocol models.Protocol) error

    // Reverse closes the given port on the router.
    Reverse(ctx context.Context, port int, protocol models.Protocol) error
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
internal/infrastructure/netbridge/
├── netbridge.go            # Netbridge implementation, builder
├── aggregate.go            # Asynx aggregate, commands, events, projection
├── allocation.go           # Port allocation logic (preferred → fallback → OS check)
├── errors.go               # Sentinel errors
├── models/
│   └── models.go           # PortAllocation, Protocol, ReadModelStore, StreamStore, Netbridge
└── strategies/
    ├── strategy.go         # Strategy interface definition
    ├── upnp.go             # UPnP implementation
    └── natpmp.go           # NAT-PMP implementation
```

Shared types live in `netbridge/models`. Both the parent `netbridge` package and the `strategies` sub-package import from `models` — no circular imports. The dependency graph is:

```
netbridge → models
netbridge → strategies
strategies → models
```

| File | Responsibility |
|------|----------------|
| `netbridge.go` | Builder, `Netbridge` struct, constructor, `Allocate` / `DeallocateByOwner` methods, strategy probing |
| `models/models.go` | `PortAllocation`, `Protocol`, `ReadModelStore` interface, `StreamStore` interface, `Netbridge` interface |
| `aggregate.go` | Internal Asynx aggregate definition, command handlers, event projection wiring |
| `allocation.go` | Port search algorithm: preferred port check, fallback range scan, OS bind test |
| `errors.go` | Package-level sentinel errors |
| `strategies/strategy.go` | `Strategy` interface |
| `strategies/upnp.go` | UPnP forwarding strategy |
| `strategies/natpmp.go` | NAT-PMP forwarding strategy |

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

    // ErrAlreadyAllocated — the specific port is already tracked in the read model.
    // This is an internal error — callers see ErrNoPortAvailable after fallback exhaustion.
    ErrAlreadyAllocated = errors.New("netbridge: port already allocated")

    // ErrBuildIncomplete — the builder is missing required dependencies.
    ErrBuildIncomplete = errors.New("netbridge: builder missing required dependencies")
)
```

Errors wrap context via `fmt.Errorf("...: %w", ErrNoPortAvailable)` so callers can use `errors.Is`.

Forwarding failures are **not** exposed as errors to the caller. They are recorded on the `PortAllocation.Forwarded` field and logged internally.

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

## 11. Open Questions

| # | Question | Default if unresolved |
|---|----------|-----------------------|
| 1 | Should Netbridge support reserving a batch of ports atomically (all-or-nothing), or is sequential allocation sufficient? | Sequential — each `Allocate` call is independent. The app layer can roll back by calling `DeallocateByOwner` if any required allocation fails. |
| 2 | Should the strategy probe run once at `Build()` time, or re-probe periodically? | Once at build time. Network topology changes mid-session are rare enough to ignore for now. |
| 3 | Should `DeallocateByOwner` also reverse router forwarding, or just free the internal allocation? | Both — reverse forwarding for each port, then deallocate. Clean up fully. |
