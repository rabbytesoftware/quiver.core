# Stuck-Runtime Recovery + Reset Escape Hatch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every runtime stuck in a transient state (`installing`, `uninstalling`, `updating`, `stopping`, `draining`) self-heal on daemon boot, and give the user a CLI escape hatch to force-reset a stuck runtime without deleting `runtime.db`.

**Architecture:** Two independent fixes. (1) Crash recovery currently only walks the arrow catalog read-model, so a stuck runtime whose namespace is not in the catalog is never reached. We add a `SELECT DISTINCT aggregate_id` enumeration to the SQLite event-store adapter and feed those namespaces into recovery alongside the catalog, so orphaned transients heal regardless of catalog state. (2) There is no way to unstick a runtime short of deleting the DB; we add `RuntimeUsecase.Reset` → `runtime.Forget` (the `Forget` primitive already exists on the repo and on `asynx`), a `DELETE /v0/runtime/:ns` endpoint, and a `quiver reset <ns>` CLI command.

**Tech Stack:** Go, `github.com/char2cs/asynx` v0.6.2 (event sourcing), GORM + `glebarez/sqlite` (event store), Gin (API), Cobra (CLI), testify.

## Global Constraints

- Module path: `github.com/rabbytesoftware/quiver.core`.
- Every new implementation must have **≥ 95% unit test coverage**; CI total gate is 90%.
- Error messages: lowercase first letter, no trailing period, colon-separated context chain, wrap sentinels with `%w`.
- App-layer sentinel errors live only in `internal/app/errors/errors.go` — do not create new ones in usecases/handlers.
- Map asynx errors (`asynxModels.ErrValidation`, `ErrPipelineFailed`) to app sentinels before returning; never let raw asynx errors reach the API layer.
- Run `make fmt` before every commit; run `make build-docs` before pushing if any swagger annotation changed (CI fails on a stale `docs/swagger/` diff).
- Linters: `funlen` ≤ 100 lines / 50 statements, `gocyclo` ≤ 15, `nestif` ≤ 2, `exhaustive` (a `default` case does NOT satisfy it), no `init()`, no mutable package-level vars.
- Branch is already `feature/cli` (targets `develop`). Keep commits on it.

## Key Facts Verified Against Source (read before starting)

- **`asynx.Asynx[T]` exposes no enumeration API** (`internal/…` — verified in `asynx@v0.6.2/asynx.go`): only `Get`, `Exists`, `Preload`, `Send`, `Forget`, etc. Enumeration must be added at quiver's own adapter layer.
- **Each aggregate type has its own event DB.** `internal/adapter/adapter.go` builds three stores: `arrow.db`, `runtime.db`, `collection.db`. `adapter.Container.RuntimeES` is the runtime one.
- **The event-store adapter is quiver-owned GORM/SQLite** (`internal/adapter/eventstore/sqlite/event_store.go`): table `events`, columns `aggregate_id` (primaryKey), `version` (primaryKey), `data`.
- **GOTCHA — the `aggregate_id` column is prefixed.** asynx's writer stores keys as `"events:"+aggregateID` (verified `asynx@v0.6.2/internal/eventstore/writer/writer.go:84`). So values look like `events:github.com/u/r@main`.
- **GOTCHA — snapshots share the same table.** quiver's `newAsynx` (`internal/app/container.go:128`) calls only `WithEventStore(es)`; asynx's builder defaults the snapshot store to the event store when none is set (`asynx@v0.6.2/builder.go:122-124`). So the same `events` table ALSO holds rows keyed `snapshots:<ns>`. **The enumeration query MUST filter to the `events:` prefix and strip it**, otherwise every aggregate is returned twice (once bare, once as a `snapshots:`-derived duplicate).
- **`runtime.Forget(ctx, ns)` already exists** (`internal/app/repositories/runtime/runtime.go:522`): checks existence, returns `nil` if absent, else `axRuntime.Forget`. Idempotent. Currently only called from `OnArrowRemoved` wiring (`internal/app/repositories/container.go:115`), which a catalog-less orphan never reaches — hence the need for a direct reset path.
- **Runtime aggregate ID == `ns.String()`** (see `GetState`/`GetRuntime` in `runtime.go`). So an enumerated aggregate ID maps directly to `domain.Namespace(id)`.
- **Recovery is invoked** from `runtimeRepository.Start` (`runtime.go:315`) → `RecoverTransients(ctx, s.listArrows, s.axRuntime, s.wizard)` in `internal/app/repositories/runtime/internal/recovery.go`.

---

## Phase 1 — Recovery scans the runtime event store (highest value)

### Task 1: Add `ListAggregateIDs` to the SQLite event-store adapter

**Files:**
- Modify: `internal/adapter/eventstore/sqlite/store.go` (interface)
- Modify: `internal/adapter/eventstore/sqlite/event_store.go` (impl)
- Test: `internal/adapter/eventstore/sqlite/event_store_test.go` (create if absent; otherwise add cases)

**Interfaces:**
- Produces: `ListAggregateIDs(ctx context.Context) ([]string, error)` on the `sqlite.Store` interface. Returns the distinct aggregate IDs currently in the store, **with the internal `events:` prefix stripped** and `snapshots:`-prefixed rows excluded. Order is unspecified.

- [ ] **Step 1: Write the failing test**

Create/extend `internal/adapter/eventstore/sqlite/event_store_test.go`:

```go
package sqlite

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStore_ListAggregateIDs_StripsPrefixAndDedupes(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	// asynx writes event rows as "events:"+id and snapshot rows as "snapshots:"+id
	// into the same table. Simulate both for two aggregates.
	require.NoError(t, store.Append(ctx, "events:github.com/u/a@v1", 1, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "events:github.com/u/a@v1", 2, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "snapshots:github.com/u/a@v1", 2, []byte(`{}`)))
	require.NoError(t, store.Append(ctx, "events:github.com/u/b@main", 1, []byte(`{}`)))

	ids, err := store.ListAggregateIDs(ctx)
	require.NoError(t, err)

	sort.Strings(ids)
	assert.Equal(t, []string{"github.com/u/a@v1", "github.com/u/b@main"}, ids)
}

func TestEventStore_ListAggregateIDs_EmptyReturnsEmpty(t *testing.T) {
	store, err := NewEventStore(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	ids, err := store.ListAggregateIDs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ids)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapter/eventstore/sqlite/ -run TestEventStore_ListAggregateIDs -v`
Expected: FAIL — `store.ListAggregateIDs undefined`.

- [ ] **Step 3: Add the method to the interface**

In `internal/adapter/eventstore/sqlite/store.go`, add to the `Store` interface:

```go
// Store is a SQLite-backed event store that extends the asynx store interface with lifecycle management.
type Store interface {
	asynxModels.Store
	io.Closer

	// ListAggregateIDs returns the distinct aggregate IDs that currently have
	// events in the store, with the internal "events:" key prefix stripped.
	ListAggregateIDs(ctx context.Context) ([]string, error)
}
```

Add `"context"` to that file's imports.

- [ ] **Step 4: Implement the method**

In `internal/adapter/eventstore/sqlite/event_store.go`, add (use `strings` — add to imports):

```go
// ListAggregateIDs returns the distinct aggregate IDs with at least one event row.
// asynx stores event keys as "events:"+id and snapshot keys as "snapshots:"+id in
// the same table, so we filter to the "events:" prefix and strip it.
func (s *eventStore) ListAggregateIDs(ctx context.Context) ([]string, error) {
	const prefix = "events:"
	var keys []string
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
		Distinct("aggregate_id").
		Where("aggregate_id LIKE ?", prefix+"%").
		Pluck("aggregate_id", &keys).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: list aggregate ids: %w", err)
	}

	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, strings.TrimPrefix(k, prefix))
	}
	return ids, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapter/eventstore/sqlite/ -run TestEventStore_ListAggregateIDs -v`
Expected: PASS (both cases).

- [ ] **Step 6: Run `make fmt` and commit**

```bash
make fmt
git add internal/adapter/eventstore/sqlite/store.go internal/adapter/eventstore/sqlite/event_store.go internal/adapter/eventstore/sqlite/event_store_test.go
git commit -m "feat(adapter): enumerate distinct aggregate ids in event store"
```

---

### Task 2: Expose the runtime aggregate lister through DI to the runtime repo

**Files:**
- Modify: `internal/adapter/container.go` (change `RuntimeES` field type so callers can reach `ListAggregateIDs`)
- Modify: `internal/app/repositories/runtime/deps.go` (add a function-type dep)
- Modify: `internal/app/repositories/runtime/runtime.go` (add field + `New` param, thread into `Start`)
- Modify: `internal/app/repositories/runtime/internal/recovery.go` (add param — see Task 3)
- Modify: `internal/app/repositories/container.go` (pass the new dep into `runtime.New`)
- Modify: `internal/app/container.go` (build the lister closure from `adapters.RuntimeES`)
- Test: `internal/app/repositories/runtime/runtime_test.go` (constructor still builds)

**Interfaces:**
- Consumes: `sqlite.Store.ListAggregateIDs` from Task 1.
- Produces:
  - `runtime.ListRuntimeAggregatesFn = func(ctx context.Context) ([]domain.Namespace, error)` (new dep type in `deps.go`).
  - `runtime.New(...)` gains a trailing `listRuntimeAggregates ListRuntimeAggregatesFn` parameter.
  - `adapter.Container.RuntimeES` (and `ArrowES`, `QuiverES` for type consistency) become type `sqlite.Store` instead of `asynxModels.Store`.

> **Ripple note:** `adapter.Container` currently types the three ES fields as `asynxModels.Store`. `sqlite.NewEventStore` already returns `sqlite.Store` (which embeds `asynxModels.Store`), and `newAsynx` accepts `asynxModels.Store`, so widening the field type to `sqlite.Store` is source-compatible everywhere it is consumed. Change all three fields together for consistency.

- [ ] **Step 1: Write the failing test**

In `internal/app/repositories/runtime/runtime_test.go`, add a compile-time check that `New` accepts the lister. Find the existing `New(...)` call in the test helpers and add a stub lister as the final argument. If there is a shared constructor helper, update it; otherwise add:

```go
func TestNew_AcceptsRuntimeAggregateLister(t *testing.T) {
	lister := func(context.Context) ([]domain.Namespace, error) { return nil, nil }
	repo, err := New(
		nil,                    // GetArrowFn
		newFakeAxRuntime(),     // asynx stub already used in this test file
		&fakeWizard{},          // wizard stub already used in this test file
		nil,                    // vault
		nil,                    // MarkInstalledFn
		nil,                    // HasDependentsFn
		nil,                    // ListArrowsFn
		domain.CurrentOS(),
		lister,                 // ListRuntimeAggregatesFn (new)
	)
	require.NoError(t, err)
	require.NotNil(t, repo)
}
```

> If the existing test file names its stubs differently, reuse those names — the point is that `New` takes the new final arg. Read `runtime_test.go` first to match the existing stub identifiers.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/repositories/runtime/ -run TestNew_AcceptsRuntimeAggregateLister -v`
Expected: FAIL — too many arguments to `New`.

- [ ] **Step 3: Add the dep type**

In `internal/app/repositories/runtime/deps.go`, add next to the existing `ListArrowsFn` declaration:

```go
// ListRuntimeAggregatesFn returns the namespaces of every runtime aggregate
// that currently has events in the runtime event store.
type ListRuntimeAggregatesFn func(ctx context.Context) ([]domain.Namespace, error)
```

Ensure `context` and `domain` are imported in `deps.go` (they are used by the sibling function types).

- [ ] **Step 4: Thread it through `New` and store the field**

In `internal/app/repositories/runtime/runtime.go`:

Add the field to `runtimeRepository`:

```go
type runtimeRepository struct {
	axRuntime             asynx.Asynx[domainRuntime.ArrowRuntime]
	wizard                wizardPkg.Wizard
	assembler             assembler.Assembler
	hasDependents         HasDependentsFn
	listArrows            ListArrowsFn
	listRuntimeAggregates ListRuntimeAggregatesFn
	drainWg               sync.WaitGroup
	drainMu               sync.Mutex
	drainClosed           bool
}
```

Add the parameter to `New` (append as the final parameter) and assign it:

```go
func New(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
	v vault.Vault,
	markInstalled MarkInstalledFn,
	hasDependents HasDependentsFn,
	listArrows ListArrowsFn,
	os domain.OS,
	listRuntimeAggregates ListRuntimeAggregatesFn,
) (Runtime, error) {
	repo := &runtimeRepository{
		axRuntime:             axRuntime,
		wizard:                w,
		assembler:             assembler.New(assembler.GetArrowFn(getArrow), axRuntime, v, nil, os),
		hasDependents:         hasDependents,
		listArrows:            listArrows,
		listRuntimeAggregates: listRuntimeAggregates,
	}

	if err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, repo.tryAddDrain); err != nil {
		return nil, fmt.Errorf("runtime: register reactions: %w", err)
	}

	return repo, nil
}
```

- [ ] **Step 5: Widen the adapter container field types**

In `internal/adapter/container.go`, change the imports and struct:

```go
import (
	// …existing…
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
)

// Container holds all adapter-layer event stores.
type Container struct {
	ArrowES   sqlite.Store
	RuntimeES sqlite.Store
	QuiverES  sqlite.Store
	closers   []io.Closer
}
```

The `asynxModels` import may become unused in this file — if `go build` reports it unused, remove it. The `closers` assignment in `New` already uses the concrete stores and needs no change.

- [ ] **Step 6: Build the lister closure and pass it down**

In `internal/app/container.go`, where `axRuntime` is built (around line 76) and the repositories container is constructed, build a lister closure from `adapters.RuntimeES` and pass it through to `repositories.New`. Add near the `axRuntime` construction:

```go
listRuntimeAggregates := func(ctx context.Context) ([]domain.Namespace, error) {
	ids, err := adapters.RuntimeES.ListAggregateIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list runtime aggregates: %w", err)
	}
	out := make([]domain.Namespace, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.Namespace(id))
	}
	return out, nil
}
```

Then add `listRuntimeAggregates` as an argument to the `repositories.New(...)` call in this file. Ensure `context`, `fmt`, and `domain` are imported.

In `internal/app/repositories/container.go`, add a matching parameter to `New(...)` (type `runtimerepo.ListRuntimeAggregatesFn`, using whatever alias the file already imports the runtime repo package under — check the import block) and forward it to the `runtime.New(...)` call as the new final argument.

- [ ] **Step 7: Run the whole affected tree**

Run: `go build ./... && go test ./internal/adapter/... ./internal/app/... -run 'TestNew_AcceptsRuntimeAggregateLister|TestEventStore' -v`
Expected: build OK; targeted tests PASS.

- [ ] **Step 8: Run `make fmt` and commit**

```bash
make fmt
git add internal/adapter/container.go internal/app/container.go internal/app/repositories/container.go internal/app/repositories/runtime/deps.go internal/app/repositories/runtime/runtime.go internal/app/repositories/runtime/runtime_test.go
git commit -m "feat(runtime): inject runtime-aggregate lister into repository"
```

---

### Task 3: Recovery scans the store, unioned with the catalog

**Files:**
- Modify: `internal/app/repositories/runtime/internal/recovery.go`
- Modify: `internal/app/repositories/runtime/runtime.go` (update `Start` call)
- Test: `internal/app/repositories/runtime/internal/recovery_test.go`

**Interfaces:**
- Consumes: `ListRuntimeAggregatesFn` (Task 2), existing `RecoverTransients` internals.
- Produces: `RecoverTransients(ctx, listArrows, listRuntimeAggregates, axRuntime, w)` — new signature. It recovers the **union** of catalog namespaces and runtime-store aggregate IDs (deduplicated), so orphaned transients with no catalog entry are healed.

> **Design:** Keep the existing catalog walk (no regression) and add the store scan, merging both into a deduplicated namespace set before running the existing per-namespace recovery switch. Store scan is a superset in principle, but the union is defensive and lets recovery degrade gracefully if either source errors.

- [ ] **Step 1: Write the failing test**

In `internal/app/repositories/runtime/internal/recovery_test.go`, add a case proving an orphan (present in the store, absent from the catalog) gets recovered. Match the existing test's stub/mocks style (read the file first; it already stubs `axRuntime` and `wizard`). Illustrative shape:

```go
func TestRecoverTransients_OrphanNotInCatalog_Recovers(t *testing.T) {
	ctx := context.Background()
	orphan := domain.Namespace("github.com/u/orphan@main")

	ax := newRecoveryFakeAx() // existing helper in this test file
	ax.setState(orphan, domain.ArrowStateInstalling)

	listArrows := func(context.Context) ([]models.ArrowView, error) {
		return nil, nil // catalog is empty — the orphan is not here
	}
	listAgg := func(context.Context) ([]domain.Namespace, error) {
		return []domain.Namespace{orphan}, nil
	}

	RecoverTransients(ctx, listArrows, listAgg, ax, &recoveryFakeWizard{})

	assert.True(t, ax.recoverInterruptedCalled(orphan),
		"orphan stuck in installing should be recovered")
}

func TestRecoverTransients_UnionDeduplicates(t *testing.T) {
	ctx := context.Background()
	ns := domain.Namespace("github.com/u/dup@v1")

	ax := newRecoveryFakeAx()
	ax.setState(ns, domain.ArrowStateInstalling)

	listArrows := func(context.Context) ([]models.ArrowView, error) {
		return []models.ArrowView{{Versions: []models.ArrowVersionView{{Namespace: ns}}}}, nil
	}
	listAgg := func(context.Context) ([]domain.Namespace, error) {
		return []domain.Namespace{ns}, nil
	}

	RecoverTransients(ctx, listArrows, listAgg, ax, &recoveryFakeWizard{})

	assert.Equal(t, 1, ax.recoverInterruptedCount(ns),
		"a namespace in both sources must be recovered exactly once")
}
```

> Read `recovery_test.go` and `internal/app/models` first to use the exact `ArrowView` / version field names (`Versions`, `Namespace`) and the existing fake-asynx helper names. The two assertions (`recoverInterruptedCalled`, `recoverInterruptedCount`) may need adding to the existing fake — extend it minimally.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/repositories/runtime/internal/ -run TestRecoverTransients -v`
Expected: FAIL — `RecoverTransients` still has the old signature / orphan not recovered.

- [ ] **Step 3: Rewrite `RecoverTransients` to union both sources**

In `internal/app/repositories/runtime/internal/recovery.go`, replace the top-level function. Extract namespace collection into a helper to stay within `funlen`/`gocyclo`:

```go
func RecoverTransients(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	listRuntimeAggregates func(ctx context.Context) ([]domain.Namespace, error),
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	w wizardPkg.Wizard,
) {
	for _, ns := range collectRecoveryNamespaces(ctx, listArrows, listRuntimeAggregates) {
		if preloadErr := axRuntime.Preload(ctx, ns.String()); preloadErr != nil {
			continue
		}
		rt, getErr := axRuntime.Get(ctx, ns.String())
		if getErr != nil || rt.Ref == "" {
			continue
		}
		switch rt.State {
		case domain.ArrowStateRunning:
			recoverRunning(ctx, ns, rt, axRuntime, w)
		case domain.ArrowStateInstalling,
			domain.ArrowStateUninstalling,
			domain.ArrowStateUpdating,
			domain.ArrowStateStopping,
			domain.ArrowStateDraining:
			sendRecoverInterrupted(ctx, ns, rt.State, axRuntime)
		case domain.ArrowStateAbsent,
			domain.ArrowStateReady,
			domain.ArrowStateDetached,
			domain.ArrowStateRemoved,
			domain.ArrowStateOutdated:
		}
	}
}

// collectRecoveryNamespaces merges catalog namespaces with runtime-store aggregate
// namespaces, deduplicated. Either source failing is logged and skipped, never fatal.
func collectRecoveryNamespaces(
	ctx context.Context,
	listArrows func(ctx context.Context) ([]models.ArrowView, error),
	listRuntimeAggregates func(ctx context.Context) ([]domain.Namespace, error),
) []domain.Namespace {
	seen := make(map[string]struct{})
	var out []domain.Namespace

	add := func(ns domain.Namespace) {
		key := ns.String()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, ns)
	}

	if items, err := listArrows(ctx); err != nil {
		slog.WarnContext(ctx, "crash recovery: list catalog", "err", err)
	} else {
		for _, vm := range items {
			for _, ver := range vm.Versions {
				add(ver.Namespace)
			}
		}
	}

	if aggs, err := listRuntimeAggregates(ctx); err != nil {
		slog.WarnContext(ctx, "crash recovery: list runtime store", "err", err)
	} else {
		for _, ns := range aggs {
			add(ns)
		}
	}

	return out
}
```

Leave `recoverRunning` and `sendRecoverInterrupted` unchanged.

- [ ] **Step 4: Update the `Start` call site**

In `internal/app/repositories/runtime/runtime.go`, update `Start`:

```go
func (s *runtimeRepository) Start(ctx context.Context) {
	runtimeinternal.RecoverTransients(ctx, s.listArrows, s.listRuntimeAggregates, s.axRuntime, s.wizard)
}
```

- [ ] **Step 5: Run recovery tests**

Run: `go test ./internal/app/repositories/runtime/... -v`
Expected: PASS (new orphan/dedupe cases and all pre-existing recovery tests).

- [ ] **Step 6: Verify coverage of recovery.go ≥ 95%**

Run: `go test ./internal/app/repositories/runtime/internal/ -cover -coverprofile=/tmp/cov.out && go tool cover -func=/tmp/cov.out | grep recovery.go`
Expected: `collectRecoveryNamespaces` and `RecoverTransients` lines covered ≥ 95%. If the both-sources-error branch is uncovered, add a case where both `listArrows` and `listRuntimeAggregates` return errors and assert no panic / no recovery calls.

- [ ] **Step 7: Run `make fmt` and commit**

```bash
make fmt
git add internal/app/repositories/runtime/internal/recovery.go internal/app/repositories/runtime/internal/recovery_test.go internal/app/repositories/runtime/runtime.go
git commit -m "fix(runtime): recover transients from the runtime store, not just the catalog"
```

---

## Phase 2 — CLI reset escape hatch

### Task 4: `RuntimeUsecase.Reset`

**Files:**
- Modify: `internal/app/usecases/runtime.go` (add `Reset` to interface + impl)
- Test: `internal/app/usecases/runtime_test.go`

**Interfaces:**
- Consumes: `runtimerepo.Runtime.Forget(ctx, ns) error` (already exists).
- Produces: `RuntimeUsecase.Reset(ctx context.Context, ns domain.Namespace) error`. Forgets the runtime aggregate (state → absent), leaving any catalog entry intact so the user can re-install. Idempotent — resetting an absent runtime returns `nil`.

> **Scope decision:** Reset forgets *only* the runtime aggregate. It deliberately does not remove the catalog arrow — the escape hatch is "unstick this so I can retry", not "delete it". A pure orphan (no catalog entry) is fully cleaned by forgetting its runtime; a diverged case (arrow lost from catalog, runtime stuck) becomes re-installable after the arrow is re-added.

- [ ] **Step 1: Write the failing test**

In `internal/app/usecases/runtime_test.go` (match the file's existing runtime-repo stub — read it first for the stub type name and how `NewRuntimeUsecase` is constructed in existing tests):

```go
func TestRuntimeUsecase_Reset_ForgetsRuntime(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &stubRuntime{} // existing stub in this test file
	uc := NewRuntimeUsecase(&stubArrow{}, rt, &stubGraph{})

	err := uc.Reset(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{ns}, rt.forgotten)
}

func TestRuntimeUsecase_Reset_PropagatesForgetError(t *testing.T) {
	ns := domain.Namespace("github.com/u/stuck@main")
	rt := &stubRuntime{forgetErr: assert.AnError}
	uc := NewRuntimeUsecase(&stubArrow{}, rt, &stubGraph{})

	err := uc.Reset(context.Background(), ns)

	require.Error(t, err)
}
```

Extend the existing `stubRuntime` with a `forgotten []domain.Namespace` recorder and `forgetErr error`, and a `Forget` method if not already present:

```go
func (s *stubRuntime) Forget(_ context.Context, ns domain.Namespace) error {
	s.forgotten = append(s.forgotten, ns)
	return s.forgetErr
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app/usecases/ -run TestRuntimeUsecase_Reset -v`
Expected: FAIL — `uc.Reset undefined`.

- [ ] **Step 3: Add `Reset` to the interface and impl**

In `internal/app/usecases/runtime.go`, add to the `RuntimeUsecase` interface (near `Stop`):

```go
	Reset(
		ctx context.Context,
		ns domain.Namespace,
	) error
```

And the implementation:

```go
// Reset forgets the runtime aggregate, clearing a runtime stuck in a transient
// state. The catalog entry (if any) is left intact so the arrow can be re-installed.
func (u *runtimeUsecase) Reset(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := u.runtime.Forget(ctx, ns); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/app/usecases/ -run TestRuntimeUsecase_Reset -v`
Expected: PASS.

- [ ] **Step 5: Run `make fmt` and commit**

```bash
make fmt
git add internal/app/usecases/runtime.go internal/app/usecases/runtime_test.go
git commit -m "feat(usecase): add RuntimeUsecase.Reset to forget a stuck runtime"
```

---

### Task 5: `DELETE /v0/runtime/:ns` endpoint

**Files:**
- Modify: `internal/api/v0/endpoints/runtime/handlers/handlers.go` (add `Reset` handler + swagger)
- Modify: `internal/api/v0/endpoints/runtime/routes.go` (register the DELETE route)
- Test: `internal/api/v0/endpoints/runtime/handlers/handlers_test.go` (add cases; match existing test package + `TestMain`)
- Regenerate: `docs/swagger/`

**Interfaces:**
- Consumes: `RuntimeUsecase.Reset` (Task 4), `libs.WriteMutationOK`, `libs.WriteErr`, `apierr.StatusAndMessage`.
- Produces: `func (h *Handlers) Reset(c *gin.Context)` → `204 No Content` on success; error status via `apierr.StatusAndMessage` on failure. Route `DELETE /runtime/:ns`.

- [ ] **Step 1: Write the failing test**

In `internal/api/v0/endpoints/runtime/handlers/handlers_test.go`, mirror the existing handler-test setup (fake usecase, gin test context). Add:

```go
func TestHandlers_Reset_Success(t *testing.T) {
	svc := &fakeRuntimeUsecase{} // existing fake in this test file
	h := New(svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "ns", Value: "github.com/u/r@main"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/runtime/github.com/u/r@main", nil)

	h.Reset(c)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, domain.Namespace("github.com/u/r@main"), svc.resetNS)
}

func TestHandlers_Reset_UsecaseError(t *testing.T) {
	svc := &fakeRuntimeUsecase{resetErr: apperrors.ErrNotFound}
	h := New(svc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "ns", Value: "github.com/u/r@main"}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/v0/runtime/github.com/u/r@main", nil)

	h.Reset(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

Extend the existing `fakeRuntimeUsecase` with `resetNS domain.Namespace`, `resetErr error`, and:

```go
func (f *fakeRuntimeUsecase) Reset(_ context.Context, ns domain.Namespace) error {
	f.resetNS = ns
	return f.resetErr
}
```

> Read the existing handler test to copy its exact namespace-parsing helper and success/error assertion pattern (there may be a shared helper for building the gin context).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/api/v0/endpoints/runtime/... -run TestHandlers_Reset -v`
Expected: FAIL — `h.Reset undefined` (and `fakeRuntimeUsecase` missing `Reset` until stub added — add the stub in Step 1).

- [ ] **Step 3: Add the handler with swagger annotations**

In `internal/api/v0/endpoints/runtime/handlers/handlers.go`, add (copy the namespace-parse + response idiom from the existing `Execute`/`Get` handlers in this file — use the same `parseNS`/`libs`/`apierr` calls they use):

```go
// Reset godoc
// @Summary      Reset a stuck runtime
// @Description  Forgets the runtime aggregate for a namespace, clearing a runtime
// @Description  stuck in a transient state (installing, uninstalling, updating, stopping).
// @Description  The catalog entry is left intact so the arrow can be re-installed.
// @Tags         runtime
// @Param        ns   path      string  true  "Arrow namespace"
// @Success      204  "runtime reset"
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /runtime/{ns} [delete]
func (h *Handlers) Reset(c *gin.Context) {
	ns, ok := parseNS(c) // use whatever the existing handlers in this file call to read+validate :ns
	if !ok {
		return
	}
	if err := h.svc.Reset(c.Request.Context(), ns); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, ns.String())
		return
	}
	libs.WriteMutationOK(c, http.StatusNoContent, ns.String())
}
```

> Match the exact namespace-extraction/validation the sibling handlers use (they already read `:ns` and validate). If they inline it rather than a `parseNS` helper, inline the same code here. Confirm `apierr`, `libs`, and `net/http` are imported (the file already imports them for other handlers).

- [ ] **Step 4: Register the route**

In `internal/api/v0/endpoints/runtime/routes.go`, add inside `Register`:

```go
	rg.DELETE("/runtime/:ns", h.Reset)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/api/v0/endpoints/runtime/... -run TestHandlers_Reset -v`
Expected: PASS.

- [ ] **Step 6: Regenerate swagger and verify no unexpected diff elsewhere**

Run: `make build-docs`
Expected: `docs/swagger/` updates to include the new DELETE route; no unrelated churn.

- [ ] **Step 7: Run `make fmt` and commit**

```bash
make fmt
git add internal/api/v0/endpoints/runtime/handlers/handlers.go internal/api/v0/endpoints/runtime/handlers/handlers_test.go internal/api/v0/endpoints/runtime/routes.go docs/swagger/
git commit -m "feat(api): DELETE /v0/runtime/:ns resets a stuck runtime"
```

---

### Task 6: `quiver reset <ns>` CLI command

**Files:**
- Modify: `internal/cli/client/methods.go` (add `ResetRuntime`)
- Modify: `internal/cli/commands/lifecycle.go` OR a small new command func in `internal/cli/commands/arrow.go`-style (add `resetCmd`)
- Modify: `internal/cli/commands/commands.go` (register `a.resetCmd()`)
- Test: `internal/cli/commands/commands_test.go` (add a registration/behavior case) and `internal/cli/client/*_test.go` if the client package has tests

**Interfaces:**
- Consumes: `DELETE /v0/runtime/:ns` (Task 5), existing `Client.do`, `a.session`, `a.confirm`, `a.withSpinner`, `validNS`, `encodeNS`.
- Produces:
  - `func (c *Client) ResetRuntime(ctx context.Context, ns string) error` → `DELETE /v0/runtime/<encodeNS(ns)>`.
  - `func (a *app) resetCmd() *cobra.Command` → `reset <namespace>`, confirmation-gated with `--yes/-y`.

- [ ] **Step 1: Write the failing test (client method)**

In the client test file (create `internal/cli/client/methods_reset_test.go` if the package uses `httptest`-based tests; match the existing test style — read `internal/cli/client/*_test.go` first):

```go
func TestClient_ResetRuntime_SendsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL) // existing constructor
	require.NoError(t, err)

	require.NoError(t, c.ResetRuntime(context.Background(), "github.com/u/r@main"))
	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/v0/runtime/github.com/u/r@main", gotPath)
}
```

> If `client.New` needs a resolved server struct rather than a URL string, mirror the exact constructor the existing client tests use.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/client/ -run TestClient_ResetRuntime -v`
Expected: FAIL — `c.ResetRuntime undefined`.

- [ ] **Step 3: Add the client method**

In `internal/cli/client/methods.go`, next to `RemoveArrow`:

```go
// ResetRuntime forgets a stuck runtime aggregate on the daemon.
func (c *Client) ResetRuntime(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodDelete, "/v0/runtime/"+encodeNS(ns), nil, nil)
}
```

- [ ] **Step 4: Run client test to verify it passes**

Run: `go test ./internal/cli/client/ -run TestClient_ResetRuntime -v`
Expected: PASS.

- [ ] **Step 5: Add the CLI command (model on `arrowRemoveCmd`)**

Add to `internal/cli/commands/lifecycle.go` (or `arrow.go` — keep it near lifecycle commands):

```go
func (a *app) resetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset <namespace>",
		Short: "Force-reset a runtime stuck in a transient state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			if err := a.confirm(cmd, yes, "reset "+args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if err := a.withSpinner(cmd, "resetting "+args[0], func() error {
				return cli.ResetRuntime(cmd.Context(), args[0])
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "reset %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
```

> Confirm `fmt` and `cobra` are imported in the chosen file (both are already used by neighboring commands).

- [ ] **Step 6: Register the command**

In `internal/cli/commands/commands.go`, add `a.resetCmd()` to the `root.AddCommand(...)` call (around line 76-77, alongside `a.uninstallCmd()`).

- [ ] **Step 7: Write + run the registration test**

In `internal/cli/commands/commands_test.go`, add a case asserting the root command exposes `reset` (match the existing pattern used to assert other subcommands are registered — read the file for the helper that walks `root.Commands()`):

```go
func TestRoot_HasResetCommand(t *testing.T) {
	root := newTestRoot(t) // existing helper that builds the root command
	assert.NotNil(t, findCommand(root, "reset"))
}
```

Run: `go test ./internal/cli/commands/ -run TestRoot_HasResetCommand -v`
Expected: PASS.

- [ ] **Step 8: Run `make fmt` and commit**

```bash
make fmt
git add internal/cli/client/methods.go internal/cli/commands/lifecycle.go internal/cli/commands/commands.go internal/cli/commands/commands_test.go internal/cli/client/
git commit -m "feat(cli): add reset command to unstick a runtime"
```

---

## Final verification (after all tasks)

- [ ] **Full gate:** `make pr-checks`
  Expected: validate-branch + fmt + vet + lint + security + build + build-docs + test-coverage + test-integration all pass. Coverage ≥ 90% total; new files ≥ 95%.
- [ ] **Manual smoke (optional, needs a daemon):** with a runtime stuck in `installing` in `runtime.db`, restart the daemon → confirm logs show `crash recovery: recovered … from installing` and `quiver ls`/`ps` show the arrow as `ready`/`absent`. Separately, force-stick one and run `quiver reset <ns>` → confirm state returns to `absent` and re-install works.

---

## Recommended follow-up plans (out of scope here)

The original diagnosis listed four intertwined root causes. This plan implements the two highest-value, self-contained fixes (recovery scan + reset). The remaining two are genuinely separate subsystems and should each get their own plan:

1. **Namespace consistency (root cause #3).** For a version-less arrow, the catalog entry is the bare namespace but operating on `…@main` creates a runtime keyed by `@main`, guaranteeing an orphan. Fix: normalize `@ref` → canonical namespace for version-less arrows at the usecase/repository boundary (or reject the `@ref`), so the runtime key always equals the catalog key. Needs its own investigation of `internal/domain/namespace.go` normalization semantics and every runtime-key call site. **Note:** the recovery scan in this plan already heals such orphans regardless, so this becomes a correctness/consistency improvement rather than a stuck-state fix.
2. **Divergence prevention (root cause #2).** Catalog projections are async and live-only (no startup replay); if the daemon dies (or the CLI's `stopIdleDaemon` reaps it) before the projection commits, the arrow is lost from `arrows.db` but survives in the event store. Fix options: flush/drain projections on daemon shutdown, and/or extend the idle-stop skip to catalog mutations (not just lifecycle commands). This also fixes "`list` doesn't show added-but-not-installed arrows". Touches `cmd/quiver/main.go`+`cli.go` shutdown ordering and the arrow store projection lifecycle.
