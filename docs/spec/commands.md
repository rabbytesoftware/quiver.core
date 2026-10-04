# Quiver — Command Catalog

## Overview

Commands are the only way state changes in this system. Each command is a pure struct that satisfies the Asynx `Command[T]` contract. The contract is purely behavioural — every command must:

- Identify the aggregate it targets (`AggregateID`).
- Validate the current aggregate snapshot against the command's intent (`Validate`). Validation must be a pure function of the input plus the current state — no I/O, no side effects, no cross-aggregate reads.
- Produce the next aggregate value (`EmitEvent`). The function must also be pure, returning a fresh value rather than mutating in place.
- Declare its event name (`EventName`) and whether the projection should write a snapshot afterwards (`ShouldSnapshot`).

All real work — git fetch, manifest parse, dependency resolution, port allocation, process launch, variable resolution, vault writes — happens in the app layer (services, use cases, reactions) before `asynx.Send()`. Commands are inert records of intent; the event log is the source of truth, and projections read events back to populate query stores.

Aggregate removal does not always require a command. The Asynx kernel exposes a `Forget(aggregateID)` operation that emits a tombstone via the projection's `OnForget` hook. Both `arrow.Remove` and `collection.Unfollow` flow through `Forget`, not through dedicated commands. Commands therefore cover state transitions inside the lifetime of the aggregate; removal is a separate kernel-level mechanism that fires `OnForget` subscribers for downstream cleanup.

The `quiver` CLI commands (`internal/cli/commands`: `install`, `arrow add`, …) are a different thing: they are the user-facing client of the HTTP API and send the commands below only indirectly. The daemon's console runs a default-deny subset of that CLI tree — see [console.md](console.md).

### Naming Convention

Event names use dot notation: `aggregate.action`. Several runtime commands share an event family — for example, every `Begin*` command emits a `runtime.begun.<namespace>` event so subscribers can listen with one regex. Commands that target a single aggregate instance suffix the namespace string to the event name (`arrow.added.github.com/org/repo@v1.0.0`), enabling per-instance `Listen` semantics. Aggregate-wide subscribers use wildcard topics (`arrow.added.*`).

| Pattern | Example |
|---|---|
| Aggregate-wide subscribe | `arrow.added.*`, `runtime.begun.*` |
| Per-instance subscribe | `runtime.ended.github.com/org/repo@v1.0.0` |
| Cross-aggregate subscribe (collection / port) | `collection.followed`, `port.Allocated` |

### Snapshot Policy

`ShouldSnapshot()` controls whether Asynx writes a snapshot row after applying the event. Snapshots speed up replay by giving projections a fast-forward starting point. The current policy is:

- **Snapshot on every command.** `ShouldSnapshot()` returns `true` unconditionally. Under asynx v0.8 a snapshot is one row upserted per aggregate (O(1) read, constant storage), so there is no cost tier left to optimise for — high-frequency commands such as step advances, PID records and port allocations snapshot too (see AGENTS.md §4.3).
The command flow is shown below; the `ShouldSnapshot` branch always takes `yes` today.

```mermaid
flowchart LR
    A[App layer: parse, resolve, prepare] --> B[Build command struct]
    B --> C[asynx.Send command]
    C --> D{Validate against current aggregate}
    D -- error --> E[Return ErrValidation to caller]
    D -- ok --> F[EmitEvent returns next aggregate]
    F --> G[Append event to event store]
    G --> H{ShouldSnapshot}
    H -- yes --> I[Write snapshot row]
    H -- no --> J[Skip snapshot]
    I --> K[Publish event on bus]
    J --> K
    K --> L[Projections, reactions, hub broadcasts]
```

---

## Arrow Commands

`Arrow` is the catalog aggregate. It carries the parsed manifest fields (meta, variables, netbridge ports, target binaries, readme), installation flags (`UserInstalled`, `InstalledAt`, `LastUsedAt`), and version state: the stored `SelectorKind`, what is installed (`Resolved{Ref, Commit, Fingerprint}`) and what is ahead (`Available`, nil when current). Arrows have no execution state — that lives on `ArrowRuntime`. The aggregate identity is the namespace string `host.tld/org/repo@selector`; the selector never changes for the life of the aggregate, so an update is an in-place `AdvanceArrow`, never a new aggregate. See [manifests/v0/versioning.md](manifests/v0/versioning.md).

Removal is performed via `axArrow.Forget(namespace)`, not a dedicated command. The repository checks `Exists` first and returns `ErrNotFound` if the aggregate is unknown. Forget triggers `OnArrowRemoved` reactions: graph dependency cleanup, runtime forget, and vault work-dir deletion.

| Command | Event Name | Snapshot | Validates | Aggregate Identity |
|---|---|---|---|---|
| `AddArrow` | `arrow.added.<ns>` | yes | `current == nil` (aggregate must not exist) | namespace |
| `SetUserInstalled` | `arrow.user_installed.<ns>` | yes | `current != nil` | namespace |
| `MarkInstalled` | `arrow.installed.<ns>` | yes | `current != nil` | namespace |
| `MarkUninstalled` | `arrow.uninstalled.<ns>` | yes | `current != nil` | namespace |
| `MarkLastUsed` | `arrow.last_used.<ns>` | yes | `current != nil` | namespace |
| `RecordAvailable` | `arrow.available_checked.<ns>` | yes | `current != nil` and `current.Resolved` equals the `Resolved` the answer was judged against | namespace |
| `RefreshManifest` | `arrow.manifest_refreshed.<ns>` | yes | `current != nil` | namespace |
| `AdvanceArrow` | `arrow.advanced.<ns>` | yes | `current != nil` | namespace |

### `AddArrow` (`arrow.added`)

Triggered when the app layer has resolved the selector against the remote, fetched the manifest at the target commit, and parsed it into the domain model — or, for an adoption, parsed manifest bytes it already holds. The command writes a fresh `Arrow` aggregate carrying meta, variables, netbridge port definitions, the per-OS targets map, the readme, the user-install flag, the `SelectorKind` and the initial `Resolved`. It rejects re-adding an existing aggregate; if the aggregate already exists the repository instead emits `SetUserInstalled` so a transitive dependency that the user later requests directly is promoted in place.

### `SetUserInstalled` (`arrow.user_installed`)

Promotes an existing arrow to user-installed status without mutating any other field. Used when a user explicitly adds an arrow that was previously pulled in only as a transitive dependency, and when an adoption lands on such a row. Validation requires the aggregate to exist.

### `MarkInstalled` / `MarkUninstalled` (`arrow.installed` / `arrow.uninstalled`)

Stamp and clear `InstalledAt`. Neither names a version: which version is on disk is `Resolved`. Fired from the post-execution hook after a successful `_install` or `_uninstall` run. Validation only requires the aggregate to exist; the lifecycle layer is responsible for ordering.

### `MarkLastUsed` (`arrow.last_used`)

Stamps `LastUsedAt` after an `_execute` run completes successfully.

### `RecordAvailable` (`arrow.available_checked`)

Records what a version check found ahead of the row (`Available`), or clears it (nil) when the row is current. The command carries the `Resolved` the check judged against, and validation refuses it when the row has moved since — an update committed between the check and the write — so an answer about a version the row has already left is never recorded. The repository re-reads and re-judges a refused write a bounded number of times, and never sends one when the answer is unchanged.

### `RefreshManifest` (`arrow.manifest_refreshed`)

Replaces meta, variables, netbridge port definitions, per-OS targets and readme in place, leaving `Resolved`, `Available` and every installation flag untouched. Sent by the update bracket to stage the target manifest before `BeginUpdate` (so the target's own `update:` steps run), and by an adoption whose manifest changed while its `Resolved` did not.

### `AdvanceArrow` (`arrow.advanced`)

Moves the row to a new version in place: replaces the manifest fields with the manifest at the target commit, sets `Resolved` to the target, and clears `Available`. Identity, runtime aggregate and workdir are untouched. Sent when an update's steps succeeded and the target is confirmed unmoved, when `PATCH /v0/arrow/{ns}` advances a row nothing is installed from, and when an adoption records a different `Resolved`. The vault manifest cache for the identity is replaced before the command is sent.

---

## ArrowRuntime Commands

`ArrowRuntime` carries the live execution context for an Arrow: the current `State` (one of `absent`, `installing`, `ready`, `running`, `stopping`, `detached`, `uninstalling`, `updating`, `outdated`, `draining`, `removed`), an optional `Execution` block (method name, step progress array, variables, PID, work directory), the most recent `LastReturn` (method, outcome, completed steps, variables snapshot), and an optional `PendingDepSync` describing dependency drift discovered while the arrow was idle.

Lifecycle methods are constants in the `domain` package: `MethodInstall`, `MethodUninstall`, `MethodUpdate`, `MethodStop`, plus the user-supplied method name passed to `BeginExecution`. Each `Begin*` command produces the event `runtime.begun.<ns>` so subscribers can observe execution start uniformly regardless of method.

| Command | Event Name | Snapshot | Validates |
|---|---|---|---|
| `BeginInstall` | `runtime.begun.<ns>` | yes | `current == nil` OR `Execution == nil` AND state is `absent`/`removed` |
| `BeginUninstall` | `runtime.begun.<ns>` | yes | state is `ready`, `Execution == nil` |
| `BeginExecution` | `runtime.begun.<ns>` | yes | `Execution == nil`; state matches `AvailableIn` (default `ready` when empty) |
| `BeginStop` | `runtime.begun.<ns>` | yes | state is `running` or `detached`; not already stopping |
| `BeginUpdate` | `runtime.begun.<ns>` | yes | state is `outdated` or `ready` |
| `EndExecution` | `runtime.ended.<ns>` | yes | `Execution != nil` |
| `AdvanceStep` | `runtime.step_advanced.<ns>` | yes | `Execution != nil` |
| `RestartExecution` | `runtime.step_advanced.<ns>` | yes | `Execution != nil` and its id is the current one |
| `RecordPID` | `runtime.pid_recorded.<ns>` | yes | `Execution != nil` |
| `RecordDetached` | `runtime.detached.<ns>` | yes | current state has a transition to `detached` |
| `RecoverInterrupted` | `runtime.recovered.<ns>` | yes | current state is transient (`installing`, `uninstalling`, `updating`, `running`, `stopping`, `draining`) |
| `MarkOutdated` | `runtime.outdated.<ns>` | yes | aggregate absent OR state is `ready` |

### `BeginInstall` (`runtime.begun`)

Starts the install lifecycle. Sent by the use case layer after the assembler resolved variables, expanded steps from the manifest, and produced a work directory. Validation accepts a fresh aggregate (first install) or an existing one whose state is `absent` or `removed` and which has no in-flight execution. The emitted aggregate sets `State = installing` and seeds `Execution` with method `_install`, the pre-resolved steps marked `pending`, the variable map, and the work directory.

### `BeginUninstall` (`runtime.begun`)

Starts the uninstall lifecycle. Validation requires `State == ready` and no in-flight execution. The emitted aggregate sets `State = uninstalling`, replaces `Execution` with the uninstall method block, and preserves `LastReturn` from the prior run for diagnostic continuity.

### `BeginExecution` (`runtime.begun`)

Starts a custom or built-in `_execute`-style method from `ready` (or another whitelist supplied via the `AvailableIn` field). Validation rejects nil aggregates and any in-flight execution. When `AvailableIn` is empty the only allowed source state is `ready`; otherwise the current state must appear in the list — the use case layer populates `AvailableIn` from `method.AvailableIn` in the manifest. The emitted aggregate sets `State = running` and seeds `Execution` with the requested method.

### `BeginStop` (`runtime.begun`)

Signals intent to halt a running or detached arrow. Validation requires `State == running` or `State == detached` and forbids re-entry while a stop is already in progress. The emitted aggregate sets `State = stopping`, replaces `Execution` with the `_stop` method block, and preserves the prior PID so the reaction can locate the process. If concurrent `AdvanceStep`/`RecordPID` writes from the running execution cause an OCC conflict, the repository retries the send up to five times before surfacing `ErrStateViolation`.

### `BeginUpdate` (`runtime.begun`)

Starts the update lifecycle. Validation requires `State == outdated` or `State == ready` (an opportunistic update is allowed even before `MarkOutdated` fires). The emitted aggregate sets `State = updating`, seeds `Execution` with the `_update` method block, preserves `LastReturn`, and explicitly clears `PendingDepSync` because the update consumes the drift the marker recorded.

### `EndExecution` (`runtime.ended`)

Terminates whatever execution is in progress and records its outcome (`success`, `failed`, `cancelled`). Validation requires `Execution != nil`. The emitted aggregate clears `Execution`, packs the just-finished method, outcome, steps, and variable snapshot into `LastReturn`, and chooses the next state via a method-and-outcome lookup: install success → `ready`, install non-success → `absent`, uninstall success → `absent`, uninstall non-success → `ready`, all other methods → `ready`.

### `AdvanceStep` (`runtime.step_advanced`)

Records that one step inside the active execution changed status (`pending → running`, `running → completed`, `running → failed`). Carries an optional error string for failed steps and an optional note for completed ones. Fires many times per execution; this is the real-time progress feed for the WebSocket hub. It snapshots like every command: a snapshot is one upserted row, so frequency costs nothing.

### `RestartExecution` (`runtime.step_advanced`)

Replaces the active execution's steps with fresh ones, all `pending`, and clears its PID; the execution's id, method, variables and work directory, the state and `LastReturn` stay. Sent by the drain goroutine when an install or update failed on a checksum mismatch and the manifest, refreshed from its host, produced different steps (see [usecases.md](./usecases.md#drain-goroutine--wizard--asynx-bridge)). It carries the `runtime.step_advanced` event name so the step-progress subscribers, and through them the WebSocket hub, republish the reset snapshot. A stale execution id is rejected with `ErrExecutionSuperseded`, like every other progress command.

### `RecordPID` (`runtime.pid_recorded`)

Captures the OS process ID that the wizard launched. Stored on the active `Execution` so a later `BeginStop` can recover it.

### `RecordDetached` (`runtime.detached`)

Transitions an arrow whose process survived a Quiver restart into the `detached` state. Validation defers to the domain state machine's `CanTransitionTo(detached)`; the emitted aggregate clears `Execution` (Quiver no longer monitors the process) but keeps `LastReturn` for diagnostics. The user must explicitly stop and restart the arrow to bring it back under Quiver's supervision.

### `RecoverInterrupted` (`runtime.recovered`)

Resets an arrow caught in a transient state at startup back to a stable state. Mapping: `installing`/`uninstalling`/`updating` → `absent` (partial work cannot be trusted); `running`/`stopping`/`draining` → `ready` (the process is gone — the alive-PID branch uses `RecordDetached` instead). Validation rejects stable states (`absent`, `ready`, `detached`, `removed`, `outdated`) so recovery is idempotent.

### `MarkOutdated` (`runtime.outdated`)

Records that a graph re-evaluation discovered dependency drift while the arrow was idle. The command carries the lists of added and removed dependency namespaces. Validation accepts either a fresh aggregate (no runtime yet) or `State == ready`; any other state means there is already work in progress and the marker would race. The emitted aggregate sets `State = outdated` and stores the drift in `PendingDepSync`. `BeginUpdate` later clears it.

---

## Collection Commands

`Collection` is the followed-quiver aggregate. It carries the namespace, the parsed `COLLECTION.md` markdown manifest, the list of arrows that failed to materialise during follow, and a `FollowedAt` timestamp. The aggregate identity is the collection namespace string. Like Arrow, removal flows through `Forget` rather than a command — `Unfollow` calls `axCollection.Forget(namespace)`, which fires `OnCollectionUnfollowed` subscribers and triggers vault cleanup.

| Command | Event Name | Snapshot | Validates |
|---|---|---|---|
| `FollowCollection` | `collection.followed` | yes | `current == nil` |

### `FollowCollection` (`collection.followed`)

Triggered when the user follows a collection through the API. The use case layer fetches and parses the markdown manifest, attempts to install each declared arrow (recording failures in `failedArrows`), and then sends this command. Validation rejects already-followed namespaces with `ErrAlreadyExists`. The emitted aggregate stamps the current time as `FollowedAt`. The event name carries no namespace suffix — the kernel routes by `AggregateID` and projections subscribe with the bare event name.

---

## PortAllocation Commands

`PortAllocation` is the netbridge engine's aggregate. Each port (TCP or UDP, specific number) is a distinct aggregate identified by the port string. The aggregate carries the port number, protocol, owner key (the consumer that holds the lease), and a flag indicating whether external port forwarding has been configured. Both commands snapshot, like every command (see Snapshot Policy).

| Command | Event Name | Snapshot | Validates |
|---|---|---|---|
| `AllocatePort` | `port.Allocated` | yes | `current == nil` or zero-valued (port currently unallocated) |
| `DeallocatePort` | `port.Deallocated` | yes | `current != nil` (port currently allocated) |

### `AllocatePort` (`port.Allocated`)

Records that a caller leased a specific port. Validation rejects the command if a non-zero allocation already exists for that port. The emitted aggregate stores the port, protocol, owner key, and forwarding flag.

### `DeallocatePort` (`port.Deallocated`)

Releases a port. Validation requires an existing allocation. The emitted aggregate is the zero value of `PortAllocation`, which the kernel treats as "available" for the next `AllocatePort`.

---

## Cross-References

- `domain.md` defines `Arrow`, `ArrowRuntime`, `Collection`, `Namespace`, `OS`, `Target`, lifecycle methods, and the runtime state machine that command validation relies on.
- `subscriptions.md` enumerates which projections, reactions, and hub broadcasts listen to each event topic.
- `usecases.md` describes the orchestration layer that resolves manifests, runs the assembler, calls dependency resolution, and finally sends the commands documented here.
- `runtime.md` and `arrow/lifecycle.md` walk through the install / execute / stop / uninstall / update flows command by command.
- `netbridge.md` explains the port allocation engine that wraps the `PortAllocation` aggregate.
