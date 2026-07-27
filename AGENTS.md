# quiver.core — Codebase Knowledge

> **How to use this file:** It describes concepts, patterns, and conventions — not current implementations. When you need exact method signatures or struct fields, read the actual source file. This file tells you WHERE to look and HOW things are wired, so you can derive the correct implementation from current code.

---

## 1. What Quiver Is

Quiver is a decentralised, event-sourced package manager. It installs and manages software through Git repositories. The `quiver.core` daemon exposes REST and WebSocket APIs for managing:

- **Arrows** — packages (manifests fetched from Git repos)
- **Collections** — curated catalogs of arrows
- **Runtimes** — execution contexts (install, run, stop, uninstall)

Module path: `github.com/rabbytesoftware/quiver.core`

---

## 2. Layer Architecture

Six layers under `internal/`, one binary at `cmd/quiver/`. **Dependencies flow strictly downward** — upper layers never import lower layers upward.

```
cmd/quiver/          ← Cobra CLI entry point
internal/internal.go ← DI wiring (New + Start)
internal/api/        ← HTTP + WebSocket delivery (Gin)
internal/app/        ← Orchestration: usecases, repositories, hub
internal/engine/     ← Stateless business engines
internal/adapter/    ← Storage backends (SQLite via Asynx + GORM)
internal/core/       ← Process singletons: config, paths, logger, fns
internal/domain/     ← Pure types and state machines (no I/O, no internal imports)
```

### Layer responsibilities

| Layer | Key rule |
|-------|----------|
| `domain/` | No I/O. No imports from other internal packages. Pure types + state machines. |
| `core/` | Config, embedded metadata, path resolution, logger, FetchNShare I/O. |
| `adapter/` | Asynx event store (SQLite) + generic `Store[T,K]` (sqlite/memory). |
| `engine/` | Manifold, Vault, Wizard, DepTree, Netbridge. Each is independent — no engine imports another. |
| `app/` | Owns Asynx aggregates, composes engines + adapters into usecases, owns `WebSocketHub`. |
| `api/` | Gin routes, Gorilla WebSocket. Maps HTTP ↔ usecase calls ↔ DTOs. Knows nothing about Asynx/commands/projections. |

### DI construction order (in `internal.New`)

```
engine.New(ctx)
adapter.New()
app.New(engines, adapters)
apiv0.New(appContainer)
api.New(appContainer.Hub, buildInfo, v0Container)
```

Read `internal/internal.go` for the current wiring.

---

## 3. Domain Model

Read files under `internal/domain/` for current struct fields — they change without notice. Below are stable descriptions of what each type represents.

### 3.1 Namespace

A string type (`domain.Namespace`) in `domain/user/repo` format, optionally with `@ref` and optionally with a 4th segment for quiver-hosted arrows (`domain/user/repo/auid`). Key operations: strip ref, extract ref, replace ref, validate, extract segments (QUID = first 3, AUID = 4th), derive clone URL.

Read `internal/domain/namespace.go` for exact methods.

### 3.2 Arrow

The canonical installed package aggregate. Holds the compiled manifest (name, description, version, variables, netbridge port definitions, per-OS targets) plus installation metadata (installed ref, constraint, timestamp).

**ArrowState** is a string enum with the following valid transitions:

| From | Can transition to |
|------|------------------|
| `absent` | `ready` |
| `ready` | `running`, `installing`, `uninstalling`, `updating`, `outdated` |
| `running` | `stopping`, `detached` |
| `stopping` | `ready`, `draining` |
| `draining` | `ready` |
| `detached` | `ready`, `stopping` |
| `installing` | `ready`, `absent` |
| `uninstalling` | `absent`, `ready` |
| `updating` | `ready`, `absent` |
| `outdated` | `ready`, `uninstalling` |
| `removed` | (terminal) |

`IsActive()` returns true for: `running`, `stopping`, `draining`, `installing`, `updating`.

### 3.3 ArrowRuntime

Execution state for an arrow. Tracks current state, active execution (method being run, progress steps, PID, workdir, variables), and last return value (outcome, steps, variables). Lives in `internal/domain/runtime/`.

### 3.4 Collection

A followed catalog of arrows. Holds namespace, follow timestamp, list of arrows in the collection, and collection metadata.

### 3.5 Target

Per-OS entry inside an Arrow manifest. Contains hardware requirements, tool/service dependency edges, exported environment values, and a lifecycle map (`install`, `update`, `execute`, `stop`, `uninstall`, user-defined methods).

### 3.6 Lifecycle method constants

```
_install   _uninstall   _update   _execute   _stop
```

These are the names passed to `wizard.Start` and appear as method keys in Arrow.Targets.

### 3.7 OS identifiers

String enum covering: `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`, `darwin/amd64`, `darwin/arm64`. `domain.CurrentOS()` returns the running platform.

---

## 4. Event Sourcing — Asynx Pattern

The project uses `github.com/char2cs/asynx`. Every state change flows through a Command.

### 4.1 Command contract

Every command struct must implement five methods. Read any file under `internal/app/repositories/*/internal/commands/` to see current examples. The five methods:

- `AggregateID() string` — returns `namespace.String()`, identifies the aggregate instance
- `EventName() string` — format: `aggregate.action.<namespace>` (e.g. `arrow.added.github.com/u/r@v1`)
- `ShouldSnapshot() bool` — whether to write a snapshot row after this event; see §4.3 (under asynx v0.8 this is unconditionally `true`)
- `Validate(current *T) error` — pure; receives current aggregate state (nil = doesn't exist yet); returns `asynxModels.ErrValidation` wrapped in context
- `EmitEvent(current *T) T` — pure; returns next aggregate value; no I/O, no logging

### 4.2 Event naming convention

Pattern: `aggregate.action.<namespace>`

Subscriber patterns use wildcards:
- `arrow.added.*` — subscribe to all instances
- `runtime.ended.github.com/user/repo@v1.0.0` — subscribe to one instance

### 4.3 Snapshot policy

A snapshot is a single upserted row keyed by `aggregate_id` (asynx v0.8) — O(1) read, constant storage, not an appended row that every future read must scan past. There is no cost tier to optimize for anymore: `ShouldSnapshot()` returns `true` unconditionally, including on high-frequency commands (`AdvanceStep`, `RecordPID`, `AllocatePort`, `DeallocatePort`).

The reader's own auto-snapshot only fires `if result.DidUpcast` (on warm and cold path alike) — it never writes a snapshot on a plain read. With no upcasters registered and `schemaVersion` at 1, that path never triggers here. The only thing that ever writes a snapshot row is a command whose `ShouldSnapshot()` is `true`.

### 4.4 Aggregate removal

Use `asynx.Forget(aggregateID)` — not a command. Triggers `OnForget` hooks for downstream cleanup.

### 4.5 Validation errors

Wrap `asynxModels.ErrValidation` with context. Never return `ErrValidation` bare.

### 4.6 Aggregate IDs are not unique across aggregate types

Arrow and Runtime commands both return `Namespace.String()` from `AggregateID()`. Their event streams stay separate *only* because each asynx instance (`internal/app/container.go: newAsynx`) has its own db file. Never point two asynx instances at the same store — their streams would interleave into one and corrupt both.

### 4.7 WorkersPerShard must stay above 1

`internal/app/container.go: newAsynx` sets `Shards: 8` and leaves `WorkersPerShard` at the asynx default. Do not drop it to 1: `DELETE /v0/arrow/:ns` → `axArrow.Forget` → the synchronous `OnArrowRemoved` cascade wired in `internal/app/repositories/container.go`, which calls `Graph.RemoveDependencies` and then `Runtime.Forget` — the latter a blocking call into `axRuntime`, which contends with the wizard drain goroutine already running inside `axRuntime` for that same aggregate. Each aggregate gets its own asynx instance and shard pool, so this is a cross-instance circular wait, not two aggregates sharing a shard — restoring more workers only widens the window instead of closing it. The real fix (making the forget-cascade non-blocking, with a boot-time reconcile for cascades that never complete) is not yet implemented. Read the full comment on `newAsynx` before touching this.

`.github/workflows/ci.yml` never runs the `integration` build tag, so this class of bug passes CI silently. Only `make pr-checks` (which runs `test-integration`) catches it — run it before any change to sharding or the forget cascade.

---

## 5. App Layer Patterns

Read `internal/app/` for current code.

### 5.1 Sentinel errors

All app-layer sentinel errors live in `internal/app/errors/errors.go`. The available sentinels are: `ErrNotFound`, `ErrAlreadyExists`, `ErrStateViolation`, `ErrMethodNotFound`, `ErrFetchFailed`, `ErrInvalidNamespace`, `ErrDependentsExist`, `ErrInvalidManifest`, `ErrPlatformNotSupported`, `ErrMissingVariable`.

Always wrap with context: `fmt.Errorf("operation name: %w", apperrors.ErrXxx)`. Never create new sentinel errors in usecases or handlers — add to `errors.go` if a new one is truly needed.

### 5.2 Usecase interface pattern

Each usecase is an exported interface + unexported struct. Constructor is exported and returns the interface. The struct holds only injected dependencies. Example structure (read actual files for current signatures):

```
type FooUsecase interface { ... }
type fooUsecase struct { dep1 ..., dep2 ... }
func NewFooUsecase(dep1, dep2) FooUsecase { return &fooUsecase{...} }
```

### 5.3 Error wrapping

Every call site wraps with context: `fmt.Errorf("operation: dep call %s: %w", ns, err)`. Chain reads outward from the error: `"install: add dep to catalog github.com/u/r@v1: not found"`.

### 5.4 Repository callback wiring

Cross-repository reactions wire in `repositories/container.go` via a `wireCallbacks()` helper. Pattern: `c.Arrow.OnArrowAdded(func(...) { c.Graph.SyncDependencies(...) })`. All wiring errors use the prefix `repositories: wire <EventName>: %w`.

### 5.5 Hub broadcast registration

WebSocket broadcasts are registered in `repositories/container.go → RegisterHubProjections`. Pattern: subscribe to an `OnRuntimeXxx` callback, call `hub.BroadcastXxx(data)` inside. Never call broadcasts from usecases.

### 5.6 Container struct pattern

Each layer (`engine`, `adapter`, `app`, `repositories`, `usecases`) has a `Container` struct with exported fields for sub-components and a `New(...)` constructor that returns `(*Container, error)`. Construction errors use prefix `<layer> container: <component>: %w`.

---

## 6. API Layer Patterns

Read `internal/api/v0/` for current code.

### 6.1 Handlers struct

Each endpoint group has a `Handlers` struct that holds the usecase interface and a `New(svc) *Handlers` constructor. Methods on the struct are the Gin handler functions.

### 6.2 Response helpers

All response writing goes through `internal/api/libs/`:
- `libs.WriteErr(c, status, msg, subject)` — error response
- `libs.WriteMutationOK(c, status, subject)` — write success (201/204)
- `libs.WriteQueryOK(c, data)` — read success (200)
- `apierr.StatusAndMessage(err)` — maps sentinel errors to HTTP codes

### 6.3 HTTP/WebSocket dispatch

The `dispatch(rest, ws gin.HandlerFunc)` helper in routes files checks for the `Upgrade: websocket` header and routes accordingly. One `GET` route can serve both.

### 6.4 Swagger annotations

Every handler function has swagger doc comments. Format: `@Summary`, `@Description`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`. Read any existing handler for the exact format. Running `make build-docs` regenerates `docs/swagger/` — CI fails if it's stale.

### 6.5 API versioning

Each version implements `Prefix() string`, `Register(*gin.RouterGroup)`, and `WSHandler() WSVersion`. New versions are passed to `api.New(...)` variadic — no changes to `api/container.go`.

---

## 7. Repository Internal Layout

```
repositories/<name>/
  <name>.go              ← public interface + constructor
  internal/
    commands/            ← one file per Asynx command
    store/               ← read-model store (GORM)
      internal/
        storage/         ← GORM models + view structs
        projections/     ← Asynx projections → update read model
    upcasters/           ← Asynx event upcasters (schema migration)
    reactions.go         ← internal Asynx subscriptions
    hooks.go             ← public callback hooks (OnXxx)
    recovery.go          ← crash recovery (runtime only)
    mocks/               ← test doubles for this repository
```

---

## 8. Go Coding Patterns

### 8.1 Error handling

- Always wrap: `fmt.Errorf("operation: %w", err)`
- Sentinel errors in `app/errors/errors.go` — check with `errors.Is`
- `isNotFound(err)` helper in repository files compares Asynx not-found by message string
- Never return bare errors from upper layers

### 8.2 Naming conventions

| Element | Pattern | Example |
|---------|---------|---------|
| Exported type | PascalCase | `ArrowState`, `TargetLifecycle` |
| Unexported struct | camelCase | `arrowUsecase`, `runtimeRepo` |
| Interface | PascalCase, no I/suffix | `ArrowUsecase`, `WebSocketHub` |
| Domain constants | descriptive PascalCase | `ArrowStateReady`, `MethodInstall` |
| Test functions | `TestType_Method_Desc` | `TestArrowState_IsActive_Running` |
| Package aliases | short camelCase | `apperrors`, `apphub`, `domainRuntime` |

### 8.3 Package aliasing

When two packages would collide on the last path segment or when the last segment is generic, alias to a short camelCase name. Common aliases: `apperrors`, `apphub`, `domainRuntime`, `domainStep`, `repoarrow`, `wizardPkg`.

### 8.4 Context propagation

`context.Context` is always the first parameter of every function that does I/O. Never store context in structs.

### 8.5 Struct construction

- Constructor: `func New(...) (*T, error)` — return nil + error on failure
- No global state mutated after process start
- Small structs: direct literal `return &Foo{field: val}`

### 8.6 Method receivers

- Pointer receiver `*T` when method mutates state or returns errors from struct fields
- Value receiver for pure computations on value types

### 8.7 JSON + YAML tags

All domain structs that cross a serialization boundary need both `yaml` and `json` tags on every field. Steps and other polymorphic types use a private `wire` struct inside `MarshalJSON`/`UnmarshalJSON` to control field naming.

### 8.8 Early returns

Prefer guard clauses over nested ifs. Validate inputs at the top, return errors early, happy path at the bottom.

---

## 9. Test Patterns

### 9.1 Test file naming and package

`<filename>_test.go` in the same directory. Domain tests: same package. API/integration tests: external package (`package foo_test`).

### 9.2 Test function naming

`TestType_Method_Description` — describes the subject, operation, and expected outcome. Examples: `TestArrowState_IsActive_RunningReturnsTrue`, `TestNamespace_Validate_EmptyReturnsError`.

### 9.3 Table-driven tests

Preferred for functions with multiple input/output cases. Variable named `testCases`, type `[]struct{ name string; ... }`. Each case has a `name` string, inputs, expected outputs. Inner `t.Run(tc.name, ...)`. Use `require` for fatal assertions (test can't proceed), `assert` for non-fatal.

### 9.4 Stubs and mocks

Simple struct implementations of interfaces — either inline in the test file or in a `mocks/` subdirectory. The project does NOT use mock-generation tools (mockery, etc.). Write stubs by hand.

### 9.5 TestMain for setup

API/Gin tests use `TestMain` to call `gin.SetMode(gin.TestMode)` before running.

### 9.6 Coverage requirement

**Every new implementation must have ≥ 95% unit test coverage.** Test all branches, all error paths, all state transitions, all edge cases (empty input, nil pointers, boundary values). CI gate is 90% total; per-implementation target is 95%.

---

## 10. Engine Interfaces

Each engine is an interface received through DI. Read the interface definitions in their respective packages for current method signatures. Summary of responsibilities:

| Engine | Package | What it does |
|--------|---------|-------------|
| `Manifold` | `engine/manifold` | Fetch + parse + compile arrow/collection manifests from namespaces |
| `Vault` | `engine/vault` | Cache manifests to disk; manage workdirs; TTL sweep |
| `Wizard` | `engine/wizard` | Spawn and supervise processes for lifecycle steps |
| `DepTree` | `engine/deptree` | Topological sort of dependency graphs |
| `Netbridge` | `engine/netbridge` | Allocate/deallocate ephemeral ports |

---

## 11. Manifest Schema (arrow@v0)

The manifest format is described in `docs/` and embedded in the binary via `metadata.yaml`. The top-level keys are `metadata`, `variables`, `netbridge`, and `targets`. Each target key is an OS string (`linux/amd64`, etc.) and contains `requirements`, `tools`, `services`, `exports`, `lifecycle`, and optional `methods`. Check `docs/` for the authoritative schema — it evolves with new manifest versions.

---

## 12. Key Flows

These describe the general call chain for major operations. Read the actual code for current details.

### Add arrow (POST /v0/arrow/:ns)

Handler validates namespace → ArrowUsecase.Add → arrow repository adds: resolves manifest via manifold, caches to vault, sends Asynx command → projection updates read model → reaction syncs dependency graph → hub broadcasts to WS clients.

### Install arrow (POST /v0/runtime/:ns/install)

RuntimeUsecase.Install → dependency graph resolves topological order → for each dep: resolve manifest if missing, register, begin install, wait for completion → begin install on target arrow → reaction starts wizard → wizard spawns process → step advance/PID events flow back → end execution event → arrow marked installed.

### Runtime reaction flow

The runtime repository subscribes to `runtime.begun.*`. On receipt: starts wizard, drains WizardEvent channel in a goroutine, translates events to Asynx commands (StepAdvanced, RecordPID, EndExecution). On successful install: calls arrow.MarkInstalled.

### WebSocket broadcast

Any Asynx projection fires a hub broadcast → Hub fans out to all Subscribers (one per API version's WS handler) → each subscriber writes to connected clients filtered by namespace glob + query params.

---

## 13. File Naming and Package Layout

- One Asynx command per file in `commands/` (e.g., `add.go`, `begin_install.go`)
- Subdirectory `internal/` inside each repository hides implementation details
- Mock interfaces in `mocks/mocks.go` (single file, multiple stubs)
- API DTOs in `api/v0/dto/<name>.go`
- Handler files: `endpoints/<resource>/handlers/handlers.go`
- Routes file: `endpoints/<resource>/routes.go`

---

## 14. Dependencies (key ones)

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/char2cs/asynx` | v0.8.x | Event sourcing kernel |
| `github.com/gin-gonic/gin` | v1.12.x | HTTP framework |
| `github.com/glebarez/sqlite` | v1.11.x | SQLite (CGo-free) |
| `github.com/gorilla/websocket` | v1.5.x | WebSocket |
| `github.com/stretchr/testify` | v1.11.x | `assert` + `require` |
| `github.com/spf13/cobra` | v1.10.x | CLI |
| `github.com/google/uuid` | v1.6.x | UUIDs |
| `gopkg.in/yaml.v3` | v3.0.x | YAML parsing |
| `gorm.io/gorm` | v1.31.x | ORM for read models |

Read `go.mod` for current versions.

---

## 15. When to Use What — Component Reference

### 15.1 Logging — `log/slog`

`logger.Init` is called once at process start. After that, call `slog` directly — no wrapper. Always use `*Context` variants (`slog.InfoContext`, `slog.WarnContext`, `slog.ErrorContext`) so logs carry the request trace. Key-value pairs as positional args after the message string (`"ns", ns, "err", err`). Only log in background goroutines and fire-and-forget callbacks where the error cannot be returned. Never log in domain types, commands, or `EmitEvent`.

**Do NOT:** create a custom logger struct, use `fmt.Println` / `log.Printf`, use third-party logging libraries.

### 15.2 Config — `internal/core/config`

Call `config.GetAPI()`, `config.GetManifold()`, `config.GetVault()`, `config.GetLogger()`, `config.GetNetbridge()`, `config.GetArrows()` at construction time. The package uses `sync.Once` — first call reads `default.yaml` (embedded) overlaid with `~/.quiver/config.yaml`. Store the resolved value in the struct; don't call `config.GetXxx()` in hot paths.

**Do NOT:** read `~/.quiver/config.yaml` manually, use `os.Getenv` for Quiver-specific config, pass config structs through layers.

### 15.3 Paths — `internal/core/paths`

Call `paths.Events()`, `paths.Store()`, `paths.Namespaces()`, `paths.Logs()` to get directories. Each call runs `os.MkdirAll` with a per-path mutex (safe for concurrent first-time creation). Test-safe variants `paths.EventsAt(homeDir)`, `paths.StoreAt(homeDir)` etc. accept a temp dir.

**Do NOT:** string-concatenate `~/.quiver/...` paths manually, call `os.MkdirAll` directly.

### 15.4 Product identity + platform metadata — `internal/core/metadata`

`metadata.GetVersion()`, `metadata.GetVersionCodename()`, `metadata.GetPlatforms()` (raw URL templates per git host), `metadata.GetVaultPath()`. Platform base URLs for GitHub, GitLab etc. come from `metadata.GetPlatforms()` — never hardcode `raw.githubusercontent.com` or similar.

**Do NOT:** hardcode git platform raw-file URLs.

### 15.5 File + HTTP I/O — `internal/core/fns`

Single API for local files and HTTP/HTTPS. `fns.Read`, `fns.Write`, `fns.Fetch`, `fns.Download`, `fns.DownloadStream`, `fns.MkdirAll`, `fns.Remove`, `fns.Copy`, `fns.Rename`, `fns.List`. Strategy dispatches automatically — file path → local strategy, `http://`/`https://` → remote strategy.

**Do NOT:** use `os.ReadFile`, `os.WriteFile`, or `http.Get` directly.

### 15.6 Manifest resolution — `engine/manifold`

Injected via DI. Use `manifold.ResolveArrow(ctx, ns)` for the full fetch+parse+compile+validate pipeline. Use `manifold.ResolveCollection(ctx, ns)` for collections. Use `manifold.ParseArrow(data)` for seeding from raw bytes. Read the interface in `internal/engine/manifold/` for current method signatures.

**Do NOT:** fetch manifests via `fns` directly, parse YAML manifest structs manually.

### 15.7 Manifest cache + workdirs — `engine/vault`

Injected via DI. Use vault to: store resolved manifests, retrieve cached manifests, allocate workdirs for executions, delete workdirs after uninstall, list cached versions. `vault.Start(ctx)` must be called (done by `engine.Container.Start`). Read the interface in `internal/engine/vault/` for current method signatures.

**Do NOT:** write files to `~/.quiver/vault/` directly, create workdirs manually.

### 15.8 Running steps — `engine/wizard`

Injected via DI. Call from inside the `runtime` repository's reaction goroutine ONLY — never from usecases or handlers. `wizard.Start(ctx, RunRequest)` returns a channel of WizardEvents. Drain the channel: step progress → AdvanceStep command, PID → RecordPID command, outcome → EndExecution command. Read `internal/engine/wizard/` for current types.

**Do NOT:** call wizard from usecases, check process state via `os.FindProcess`.

### 15.9 Dependency graph — `app/repositories/graph`

From usecases, always use the app-layer `graph.Graph` (not `engine/deptree` directly). Methods: `Resolve(ctx, ns)` for topological order, `HasDependents(ctx, ns, exclude)`, `GetDependents(ctx, ns)`, `DiffDeps(current, next)`. Read the interface in `internal/app/repositories/graph/`.

**Do NOT:** call `deptree.DepTree` directly from usecases, implement your own topological sort.

### 15.10 Port allocation — `engine/netbridge`

Called internally by the variable assembler when resolving port variables. You will rarely call this directly. New features needing ports should declare them as `netbridge` entries in the manifest.

### 15.11 Arrow catalog — `app/repositories/arrow`

Injected into usecases. Methods include: `Get`, `Exists`, `List`, `Add` (triggers manifold resolve), `Remove`, `UpdateManifest`, `MarkInstalled`, and event hooks (`OnArrowAdded`, `OnArrowUpdated`, `OnArrowRemoved`, `OnArrowUpgraded`). Read the interface in `internal/app/repositories/arrow/arrow.go`.

**Do NOT:** call `asynx.Send` directly from usecases, read `domain.Arrow` from Asynx directly.

### 15.12 Runtime execution state — `app/repositories/runtime`

Injected into usecases. Methods include: `GetState`, `GetRuntime`, `BeginInstall`, `BeginExecution`, `BeginStop`, `BeginUninstall`, `BeginUpdate`, `MarkOutdated`, `ListenEnded`, and event hooks. Read the interface in `internal/app/repositories/runtime/runtime.go`.

**Do NOT:** check process state via `os.FindProcess`, subscribe to Asynx topics for runtime events from usecases.

### 15.13 WebSocket broadcasts — `app/hub`

Injected into repositories. Fire-and-forget. Three broadcast methods, each accepting a typed event wrapper. `CatalogUpserted` = add/update, `CatalogRemoved` = delete/unfollow. Registered in `repositories/container.go → RegisterHubProjections`.

**Do NOT:** write to WS connections directly from repositories or usecases, call broadcasts from usecases.

### 15.14 Error type → HTTP status — `api/libs/apierr`

`apierr.StatusAndMessage(err)` maps app-layer sentinel errors to HTTP status codes. Always use this — don't hard-code status codes in handlers. Read `internal/api/libs/apierr/` for the current mapping.

### 15.15 Summary table

| Task | Package |
|------|---------|
| Structured logging | `log/slog` (stdlib) |
| Read config values | `internal/core/config` |
| Resolve named directories | `internal/core/paths` |
| Product identity / platform URLs | `internal/core/metadata` |
| File + HTTP I/O | `internal/core/fns` |
| Parse + compile arrow manifest | `internal/engine/manifold` |
| Cache manifest / workdir | `internal/engine/vault` |
| Run lifecycle steps | `internal/engine/wizard` (runtime repo only) |
| Topological dep sort | `internal/app/repositories/graph` |
| Port allocation | `internal/engine/netbridge` (via assembler) |
| Read/write arrow catalog | `internal/app/repositories/arrow` |
| Read/write runtime state | `internal/app/repositories/runtime` |
| Broadcast to WS clients | `internal/app/hub` |
| Error type → HTTP status | `internal/api/libs/apierr` |

---

## 16. Code Style and Formatting

### 16.1 Formatters (mandatory, enforced by CI)

Two formatters run via `make fmt`:

1. **`gofumpt`** with `extra-rules: true` — stricter than `gofmt`. Enforces blank lines between top-level declarations, grouped blocks, removes unnecessary blank lines inside functions.
2. **`goimports`** with `local-prefixes: github.com/rabbytesoftware/quiver` — groups imports into three blocks: (1) stdlib, (2) third-party, (3) internal. **Never manually re-order imports.** Always run `make fmt`.

### 16.2 Linter rules (golangci-lint)

| Linter | What it enforces |
|--------|-----------------|
| `errcheck` | All error return values must be checked |
| `exhaustive` | Switch on named types must cover all values; `default` does NOT count |
| `funlen` | Max 100 lines / 50 statements per function |
| `gochecknoinits` | `init()` functions are banned |
| `gocyclo` | Cyclomatic complexity ≤ 15 |
| `gosec` | Security patterns (except G404) |
| `nestif` | Nested `if` complexity ≤ 2 |
| `revive/early-return` | Prefer early returns over deep nesting |
| `staticcheck` | Go static analysis |
| `unused` | No unexported unused symbols |
| `whitespace` | No unnecessary blank lines |

Test files are exempt from `funlen`, `gocyclo`, `errcheck`, `gosec`. Suppression: `//nolint:lintername` on the specific line with a comment explaining why.

### 16.3 No `init()` functions

`gochecknoinits` bans `init()`. **Why:** `init()` creates hidden global state causing non-deterministic tests. Use constructors + functional options instead. Every component receives its config through `New(...)`.

### 16.4 No mutable package-level variables

Only allowed at package level: `errors.New(...)` sentinel errors, typed constants, compile-time interface checks (`var _ Interface = (*Impl)(nil)`). Never declare mutable state at package level — it causes test interference.

### 16.5 Functional options pattern

Used to make constructors testable without coupling to the process environment. Pattern: `type Option func(*opts)` + `func WithHomeDir(dir string) Option`. Allows tests to pass a temp dir: `engine.New(ctx, engine.WithHomeDir(t.TempDir()))`. Every layer has its own `Option` type.

### 16.6 Comment style

- Doc comments on exported symbols: single sentence starting with the symbol name, period at end.
- Inline comments: only when WHY is non-obvious. Never explain what the code does.
- No TODO/FIXME in new code. No commented-out code. No "added for X"/"used by Y" annotations.
- English only. No Spanish in source files.

### 16.7 Function length

Hard limits: 100 lines / 50 statements. When over the limit, extract a private helper — do not just add `//nolint:funlen`.

### 16.8 Switch exhaustiveness

`default-signifies-exhaustive: false` — a `default` case does NOT satisfy the `exhaustive` linter. Either cover all cases explicitly, or add `//nolint:exhaustive` with justification.

### 16.9 Error message format

Lowercase first letter, no trailing period, colon-separated context chain: `"outer operation: inner call: leaf error"`. Wrap with `%w` to preserve the sentinel chain. Never expose internal types or stack traces in user-facing errors.

### 16.10 Asynx error mapping

Always map Asynx errors to app-layer sentinels before returning. `asynxModels.ErrValidation` and `ErrPipelineFailed` → `apperrors.ErrStateViolation`. Never let raw Asynx errors propagate to the API layer.

---

## 17. Makefile Commands

Run from `quiver.core/` root.

### Build

| Command | What it does |
|---------|-------------|
| `make build` | clean + deps + fmt + vet → binary at `bin/quiver` with CGO_ENABLED=0 and version/buildID ldflags |
| `make run` | `go run ./cmd/quiver daemon` — no binary |
| `make clean` | Delete `bin/` and `coverage/` |

### Dependencies

| Command | What it does |
|---------|-------------|
| `make deps` | `go mod download` + verify + tidy |
| `make setup` | `go mod download` + verify + create `bin/` |
| `make install-tools` | Install dev tools: golangci-lint, gofumpt, goimports, swag, asyncapi-cli |

### Quality

| Command | What it does |
|---------|-------------|
| `make fmt` | Run gofumpt then goimports. Run before every commit. |
| `make vet` | `go vet ./...` |
| `make lint` | `golangci-lint run ./...` |
| `make security` | Install + run gosec |

### Testing

| Command | What it does |
|---------|-------------|
| `make test` | All unit tests with race detector |
| `make test-coverage` | Unit tests + write coverage.out + HTML report + per-function stdout |
| `make test-integration` | Integration tests (live system, slower) |
| `make test-all` | Both test and test-integration |
| `make test-docker` | Full suite in golang-alpine container (clean-room check) |
| `make bench` | Integration benchmarks with regression check vs baseline.json (1.25× threshold) |
| `make bench-update` | Regenerate benchmark baseline |

### Coverage helpers

| Command | What it does |
|---------|-------------|
| `make missing-tests` | List source files with no `_test.go` companion |
| `make coverage-files COVERAGE_BELOW=40` | List files with coverage below N% |
| `make coverage-funcs COVERAGE_BELOW=40` | Same, per-function granularity |

### Docs + CI

| Command | What it does |
|---------|-------------|
| `make build-docs` | Regenerate docs/swagger/ + validate asyncapi.yaml |
| `make pr-checks` | Full gate: validate-branch + clean + deps + fmt + vet + lint + security + build + build-docs + test-coverage + test-integration + bench |
| `make validate-branch` | Fail if branch name doesn't match convention |

### asdf + Go tools note

asdf sets `GOBIN` to a version-specific path that differs from `GOPATH/bin`. The Makefile uses `$(go env GOPATH)/bin`. If `make fmt` or `make build` fail with "No such file or directory" for gofumpt or goimports:

```bash
GOBIN=$(go env GOPATH)/bin make install-tools
```

This only affects the current Go version's packages directory. Re-run when switching Go versions.

---

## 18. Branch Naming and PR Process

### Branch naming

| Prefix | Use for | PR target |
|--------|---------|-----------|
| `feature/<name>` | New functionality | `develop` |
| `enhancement/<name>` | Improvement to existing | `develop` |
| `fix/<name>` | Bug fix | `develop` |
| `refactor/<name>` | Refactor, no behaviour change | `develop` |
| `hotfix/<name>` | Urgent production fix | `master` |
| `release/<name>` | Release cut | `develop` → `master` |
| `dependabot/<name>` | Automated dep bumps | `develop` |

**Format:** `kebab-case` — all lowercase, hyphens only. No camelCase, no underscores, no username prefix.

`make validate-branch` enforces this. CI adds two extra accepted prefixes not in the Makefile: `beta/<version>` (pre-release) and `backport/<name>` (backport).

### PR process

1. Run `make pr-checks` locally before opening the PR
2. Do NOT open as draft — CI skips draft PRs
3. Fill the PR template (Description, Type, Related Issues, Changes, Breaking Changes, Checklist)
4. CI pipeline: `validate-branch` → `code-quality` + `test-coverage` + `build` → `test-multi-platform`
5. **Coverage gate:** 90% total. **Format gate:** any `make fmt` diff fails CI. **Swagger gate:** any `make build-docs` diff fails CI.

---

## 19. Common Pitfalls

1. Never import a higher layer from a lower one. `domain` imports nothing internal.
2. Never bypass Asynx. State changes must go through commands → `asynx.Send()`.
3. Never put business logic in commands. `Validate` and `EmitEvent` are pure; I/O happens before `asynx.Send()`.
4. Don't use `asynx.Forget` for state transitions — only for permanent removal.
5. Don't create new sentinel errors in usecases/handlers — add to `app/errors/errors.go` only.
6. Always propagate `context.Context` cancellation. Check `ctx.Done()` in blocking selects.
7. Always validate that a namespace has a ref (`ns.Ref() == ""`) before operations that require a versioned namespace.
8. Both `yaml` and `json` tags required on all fields of types that cross serialization boundaries.
9. Test coverage ≥ 95% per new implementation. No exceptions.
10. Error messages: lowercase first letter, no trailing punctuation.
11. No `init()` functions.
12. No mutable package-level variables.
13. Map Asynx errors to app sentinels before returning.
14. Run `make fmt` before every commit. Run `make build-docs` before pushing if any handler or swagger annotation changed.
15. Don't suppress linter warnings without a justification comment.
