# Integration Test Speedup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce integration test CI time from 10+ minutes to under 2 minutes by silencing verbose logging, cutting fixture sleep times, and enabling parallel suite execution.

**Architecture:** Two independent phases. Phase 1 silences GORM's SQL logging (which currently dumps 10KB+ JSON blobs per event-store insert) and trims the service-b fixture sleep — both are single-line changes with immediate impact. Phase 2 threads a `homeDir` override through the engine/adapter/app constructors so tests no longer mutate the process-level `HOME` env var, which is the only blocker to running test suites in parallel.

**Tech Stack:** Go 1.24, GORM, testify/suite, SQLite (glebarez), GitHub Actions ubuntu-latest.

---

## Diagnosis Summary

The CI run takes 10+ minutes because of three compounding problems:

1. **GORM debug logging is ON** (`&gorm.Config{}` uses GORM's default logger). Each event-store INSERT logs a ~10KB JSON blob. Under concurrent adds, this is hundreds of multi-kilobyte log lines written to stdout, adding severe I/O pressure on CI's 2-core runner.

2. **`TestConcurrency_ConcurrentInstallsSharedDep` times out** — composed-c stays at `absent` (30s × 2 waitForState = 60s wasted). The orphaned cleanup goroutines then compete with subsequent tests for CPU, making every deps test that installs service-b run 20–30× slower than locally.

3. **Tests run sequentially.** `s.T().Setenv("HOME", home)` mutates a process-level env var, preventing parallel execution of test suites.

---

## File Map

| File | Change |
|---|---|
| `internal/adapter/store/sqlite/sqlite.go` | Silence GORM logger in all 3 gorm.Open calls |
| `internal/adapter/eventstore/sqlite/event_store.go` | Silence GORM logger |
| `tests/integration/testdata/arrows/service-b/arrow.yaml` | sleep 60 → sleep 5, timeout 70s → 15s |
| `internal/core/metadata/metadata.go` | Add `GetEventsPathAt`, `GetStorePathAt` |
| `internal/core/paths/paths.go` | Add `EventsAt(homeDir)`, `StoreAt(homeDir)` |
| `internal/engine/container.go` | Accept `WithHomeDir` option; use `paths.EventsAt` |
| `internal/adapter/container.go` | Accept `WithHomeDir` option; use `paths.EventsAt` |
| `internal/app/arrow/builder.go` | Add `WithHomeDir`; use `paths.StoreAt` in `Build()` |
| `internal/app/quiver/builder.go` | Add `WithHomeDir`; use `paths.StoreAt` in `Build()` |
| `internal/app/container.go` | Accept `WithHomeDir`, pass it to both builders |
| `tests/integration/env_test.go` | Use `engine.WithHomeDir`, `adapter.WithHomeDir`, `app.WithHomeDir`; remove `s.T().Setenv` |
| `tests/integration/suite_test.go` | Split into 5 parallel-capable top-level `TestXxxIntegration` functions |
| `Makefile` | Use `-parallel 5` on `test-integration` |

---

## Task 1: Silence GORM Logging

**Files:**
- Modify: `internal/adapter/store/sqlite/sqlite.go`
- Modify: `internal/adapter/eventstore/sqlite/event_store.go`

This is the single highest-impact change. GORM's default logger emits every SQL query. Each event-store INSERT contains the full serialized event JSON (often 5–10KB). Silencing it eliminates the massive I/O bottleneck.

- [ ] **Step 1: Add silent logger to sqlite store**

In `internal/adapter/store/sqlite/sqlite.go`, change every `&gorm.Config{}` to include a silent logger. There are two occurrences: in `New` and in `OpenDB`.

```go
import (
    // existing imports...
    "gorm.io/gorm/logger"
)

// In New[T, K]:
db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Silent),
})

// In OpenDB:
db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Silent),
})

// In NewFromDB: no gorm.Open call here, DB is passed in — no change needed.
```

- [ ] **Step 2: Add silent logger to event store**

In `internal/adapter/eventstore/sqlite/event_store.go`:

```go
import (
    // existing imports...
    "gorm.io/gorm/logger"
)

// In NewEventStore:
db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
    Logger: logger.Default.LogMode(logger.Silent),
})
```

- [ ] **Step 3: Verify no GORM output during tests**

Run: `go test -tags integration -run TestConcurrency_AddSameNamespace ./tests/integration/ -v 2>&1 | grep -i "INSERT\|SELECT\|UPDATE" | wc -l`

Expected: `0` (no SQL output)

- [ ] **Step 4: Commit**

```bash
git add internal/adapter/store/sqlite/sqlite.go \
        internal/adapter/eventstore/sqlite/event_store.go
git commit -m "fix(sqlite): silence GORM SQL logging — eliminates 10KB-per-insert CI I/O"
```

---

## Task 2: Reduce service-b Execute Sleep

**Files:**
- Modify: `tests/integration/testdata/arrows/service-b/arrow.yaml`

The execute step runs `sleep 60` with a 70s timeout. Tests that stop service-b (ServiceStop, RemoveAfterDependentsGone, OrphanCleanup) only need the process to be alive long enough to transition to `running` state. 5 seconds is more than sufficient.

- [ ] **Step 1: Shorten sleep and timeout**

```yaml
# tests/integration/testdata/arrows/service-b/arrow.yaml
# Under targets."*".lifecycle.execute:
- type: run
  command: sleep 5
  title: Execute
  timeout: 15s
  exit_on_failure: true
```

All other lifecycle steps (install, stop, uninstall) remain unchanged.

- [ ] **Step 2: Verify tests that need service-b still pass locally**

Run: `go test -tags integration -run 'TestLifecycle_ServiceStop|TestDeps_TransitiveInstall|TestDeps_OrphanCleanup' ./tests/integration/ -v -timeout 120s`

Expected: all 3 tests PASS

- [ ] **Step 3: Commit**

```bash
git add tests/integration/testdata/arrows/service-b/arrow.yaml
git commit -m "test(integration): reduce service-b sleep from 60s to 5s"
```

---

## Task 3: Add homeDir Override to metadata and paths

**Files:**
- Modify: `internal/core/metadata/metadata.go`
- Modify: `internal/core/paths/paths.go`

`metadata.GetEventsPath()` and `GetStorePath()` call `resolveHome()` which reads the `HOME` env var. Adding `GetEventsPathAt(homeDir)` and `GetStorePathAt(homeDir)` variants allows callers to supply an explicit home directory without touching the process environment.

- [ ] **Step 1: Add path-at functions to metadata**

In `internal/core/metadata/metadata.go`, add after the existing `GetEventsPath` and `GetStorePath` functions:

```go
// GetEventsPathAt returns the events directory path under an explicit homeDir,
// bypassing the process-level HOME env var. Used by tests for parallel isolation.
func GetEventsPathAt(homeDir string) string {
    return resolvePath(Get().Paths.Events, homeDir)
}

// GetStorePathAt returns the store directory path under an explicit homeDir,
// bypassing the process-level HOME env var. Used by tests for parallel isolation.
func GetStorePathAt(homeDir string) string {
    return resolvePath(Get().Paths.Store, homeDir)
}
```

- [ ] **Step 2: Add path-at functions to paths package**

In `internal/core/paths/paths.go`, add after the existing `Events` and `Store` functions:

```go
// EventsAt returns (and creates) the event-store directory under an explicit homeDir.
func EventsAt(homeDir string) (string, error) {
    return ensure(metadata.GetEventsPathAt(homeDir))
}

// StoreAt returns (and creates) the catalog store directory under an explicit homeDir.
func StoreAt(homeDir string) (string, error) {
    return ensure(metadata.GetStorePathAt(homeDir))
}
```

- [ ] **Step 3: Verify metadata tests still pass**

Run: `go test ./internal/core/metadata/... ./internal/core/paths/... -v`

Expected: all existing tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/core/metadata/metadata.go internal/core/paths/paths.go
git commit -m "feat(paths): add EventsAt/StoreAt variants for explicit homeDir override"
```

---

## Task 4: Thread homeDir Through engine.New and adapter.New

**Files:**
- Modify: `internal/engine/container.go`
- Modify: `internal/adapter/container.go`

Add a functional `WithHomeDir` option to both containers. When provided, they use `paths.EventsAt(homeDir)` instead of `paths.Events()`.

- [ ] **Step 1: Add WithHomeDir to engine container**

Replace `internal/engine/container.go`:

```go
package engine

import (
    "context"
    "fmt"
    "path/filepath"
    "time"

    sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
    "github.com/rabbytesoftware/quiver/internal/core/config"
    "github.com/rabbytesoftware/quiver/internal/core/paths"
    "github.com/rabbytesoftware/quiver/internal/engine/deptree"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold"
    "github.com/rabbytesoftware/quiver/internal/engine/netbridge"
    "github.com/rabbytesoftware/quiver/internal/engine/vault"
    "github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Container holds all engine-layer dependencies.
type Container struct {
    Vault     vault.Vault
    Manifold  manifold.Manifold
    Wizard    wizard.Wizard
    Netbridge netbridge.Netbridge
    DepTree   deptree.DepTree
}

type containerOpts struct{ homeDir string }

// Option configures engine.New.
type Option func(*containerOpts)

// WithHomeDir overrides the home directory used for path resolution.
// When not set, paths resolve from the process-level HOME env var.
func WithHomeDir(dir string) Option {
    return func(o *containerOpts) { o.homeDir = dir }
}

// New constructs all engines and returns a ready-to-use Container.
func New(ctx context.Context, opts ...Option) (*Container, error) {
    cfg := containerOpts{}
    for _, o := range opts {
        o(&cfg)
    }

    var (
        eventsPath string
        err        error
    )
    if cfg.homeDir != "" {
        eventsPath, err = paths.EventsAt(cfg.homeDir)
    } else {
        eventsPath, err = paths.Events()
    }
    if err != nil {
        return nil, fmt.Errorf("engine container: %w", err)
    }

    es, err := sqlite.NewEventStore(filepath.Join(eventsPath, "netbridge.db"))
    if err != nil {
        return nil, fmt.Errorf("engine container: %w", err)
    }

    nb, err := netbridge.New().WithEventStore(es).Build(ctx)
    if err != nil {
        return nil, fmt.Errorf("engine container: netbridge: %w", err)
    }

    wiz, err := wizard.New()
    if err != nil {
        return nil, fmt.Errorf("engine container: wizard: %w", err)
    }

    fetchTimeout, err := time.ParseDuration(config.GetManifold().FetchTimeout)
    if err != nil {
        fetchTimeout = 30 * time.Second
    }

    return &Container{
        Vault:     vault.New("", 0),
        Manifold:  manifold.New(fetchTimeout),
        Wizard:    wiz,
        Netbridge: nb,
        DepTree:   deptree.New(),
    }, nil
}
```

- [ ] **Step 2: Add WithHomeDir to adapter container**

Replace `internal/adapter/container.go`:

```go
package adapter

import (
    "fmt"
    "path/filepath"

    asynxModels "github.com/char2cs/asynx/models"
    "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
    "github.com/rabbytesoftware/quiver/internal/core/paths"
)

type Container struct {
    ArrowES   asynxModels.Store
    RuntimeES asynxModels.Store
    QuiverES  asynxModels.Store
}

type adapterOpts struct{ homeDir string }

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(dir string) Option {
    return func(o *adapterOpts) { o.homeDir = dir }
}

func New(opts ...Option) (*Container, error) {
    cfg := adapterOpts{}
    for _, o := range opts {
        o(&cfg)
    }

    var (
        eventsPath string
        err        error
    )
    if cfg.homeDir != "" {
        eventsPath, err = paths.EventsAt(cfg.homeDir)
    } else {
        eventsPath, err = paths.Events()
    }
    if err != nil {
        return nil, fmt.Errorf("adapter: %w", err)
    }

    arrowES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "arrow.db"))
    if err != nil {
        return nil, fmt.Errorf("adapter: arrow event store: %w", err)
    }

    runtimeES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "runtime.db"))
    if err != nil {
        return nil, fmt.Errorf("adapter: runtime event store: %w", err)
    }

    quiverES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "quiver.db"))
    if err != nil {
        return nil, fmt.Errorf("adapter: quiver event store: %w", err)
    }

    return &Container{
        ArrowES:   arrowES,
        RuntimeES: runtimeES,
        QuiverES:  quiverES,
    }, nil
}
```

- [ ] **Step 3: Confirm compilation**

Run: `go build ./internal/engine/... ./internal/adapter/...`

Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/engine/container.go internal/adapter/container.go
git commit -m "feat(engine,adapter): accept WithHomeDir option for test-parallel path isolation"
```

---

## Task 5: Thread homeDir Through app.New and Its Builders

**Files:**
- Modify: `internal/app/arrow/builder.go`
- Modify: `internal/app/quiver/builder.go`
- Modify: `internal/app/container.go`

Both app builders call `paths.Store()`. They need a `WithHomeDir` method to use `paths.StoreAt` instead.

- [ ] **Step 1: Add WithHomeDir to arrow builder**

In `internal/app/arrow/builder.go`, add a `homeDir` field to `Builder` and a `WithHomeDir` method. Then update the `Build` method to use it:

```go
// Add field to Builder struct:
type Builder struct {
    // ... existing fields ...
    homeDir string
}

// Add method:
func (b *Builder) WithHomeDir(dir string) *Builder {
    b.homeDir = dir
    return b
}

// In Build(), replace:
//   storePath, storePathErr := paths.Store()
// with:
var storePath string
var storePathErr error
if b.homeDir != "" {
    storePath, storePathErr = paths.StoreAt(b.homeDir)
} else {
    storePath, storePathErr = paths.Store()
}
```

- [ ] **Step 2: Add WithHomeDir to quiver builder**

In `internal/app/quiver/builder.go`, same pattern:

```go
type Builder struct {
    // ... existing fields ...
    homeDir string
}

func (b *Builder) WithHomeDir(dir string) *Builder {
    b.homeDir = dir
    return b
}

// In Build(), replace paths.Store() with:
var storePath string
var storePathErr error
if b.homeDir != "" {
    storePath, storePathErr = paths.StoreAt(b.homeDir)
} else {
    storePath, storePathErr = paths.Store()
}
```

- [ ] **Step 3: Thread homeDir through app.New**

In `internal/app/container.go`, add options:

```go
type appOpts struct{ homeDir string }

// Option configures app.New.
type Option func(*appOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(dir string) Option {
    return func(o *appOpts) { o.homeDir = dir }
}

func New(
    engines *engine.Container,
    adapters *adapter.Container,
    hub apphub.WebSocketHub,
    opts ...Option,
) (*Container, error) {
    cfg := appOpts{}
    for _, o := range opts {
        o(&cfg)
    }

    os := domain.CurrentOS()

    arrowBuilder := arrow.NewArrowBuilder().
        WithEngines(engines).
        WithEventStore(adapters.ArrowES).
        WithRuntimeEventStore(adapters.RuntimeES).
        WithOS(os).
        WithWebSocketHub(hub)
    if cfg.homeDir != "" {
        arrowBuilder = arrowBuilder.WithHomeDir(cfg.homeDir)
    }
    arrowSvc, err := arrowBuilder.Build()
    if err != nil {
        return nil, fmt.Errorf("app container: arrow: %w", err)
    }

    quiverBuilder := quiver.NewQuiverBuilder().
        WithEngines(engines).
        WithEventStore(adapters.QuiverES).
        WithWebSocketHub(hub)
    if cfg.homeDir != "" {
        quiverBuilder = quiverBuilder.WithHomeDir(cfg.homeDir)
    }
    quiverSvc, err := quiverBuilder.Build()
    if err != nil {
        return nil, fmt.Errorf("app container: quiver: %w", err)
    }

    return &Container{Arrow: arrowSvc, Quiver: quiverSvc}, nil
}
```

- [ ] **Step 4: Confirm compilation**

Run: `go build ./internal/app/...`

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/app/arrow/builder.go \
        internal/app/quiver/builder.go \
        internal/app/container.go
git commit -m "feat(app): accept WithHomeDir option — enables parallel integration test isolation"
```

---

## Task 6: Wire homeDir Into Tests and Enable Parallel Suites

**Files:**
- Modify: `tests/integration/env_test.go`
- Modify: `tests/integration/suite_test.go`

Remove `s.T().Setenv("HOME", home)` (process-global mutation) and replace it with `engine.WithHomeDir`, `adapter.WithHomeDir`, and `app.WithHomeDir`. Then split the single `TestIntegration` suite into 5 parallel-capable top-level functions.

- [ ] **Step 1: Rewrite env_test.go to pass homeDir via options**

```go
//go:build integration

package integration_test

import (
    "context"
    "net/http/httptest"

    "github.com/rabbytesoftware/quiver/internal/adapter"
    "github.com/rabbytesoftware/quiver/internal/api"
    "github.com/rabbytesoftware/quiver/internal/app"
    "github.com/rabbytesoftware/quiver/internal/engine"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold"
)

type Env struct {
    URL   string
    home  string
    close func()
}

func (e *Env) Close() { e.close() }

func (s *IntegrationSuite) buildEnv(home string) *Env {
    ctx, cancel := context.WithCancel(context.Background())

    engines, err := engine.New(ctx, engine.WithHomeDir(home))
    s.Require().NoError(err)

    rsv := &testResolver{repos: s.repos}
    engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

    adapters, err := adapter.New(adapter.WithHomeDir(home))
    s.Require().NoError(err)

    hub := api.NewHub()
    appContainer, err := app.New(engines, adapters, hub, app.WithHomeDir(home))
    s.Require().NoError(err)

    apiContainer, err := api.New(appContainer, hub)
    s.Require().NoError(err)

    srv := httptest.NewServer(apiContainer)
    closeAll := func() {
        srv.Close()
        cancel()
    }
    env := &Env{URL: srv.URL, home: home, close: closeAll}
    s.T().Cleanup(env.close)
    return env
}

func (s *IntegrationSuite) newEnv() *Env {
    return s.buildEnv(s.T().TempDir())
}

func (s *IntegrationSuite) newEnvWithHome(home string) *Env {
    return s.buildEnv(home)
}
```

- [ ] **Step 2: Split suite_test.go into parallel top-level functions**

Replace `tests/integration/suite_test.go`:

```go
//go:build integration

package integration_test

import (
    "testing"

    "github.com/stretchr/testify/suite"
)

// IntegrationSuite is the shared base embedded by all suites.
// It builds fixture repos once per suite in SetupSuite.
type IntegrationSuite struct {
    suite.Suite
    repos fixtureRepos
}

func (s *IntegrationSuite) SetupSuite() {
    s.repos = buildFixtureRepos(s.T())
}

// LifecycleSuite runs lifecycle tests in parallel with other suites.
type LifecycleSuite struct{ IntegrationSuite }
type DepsSuite struct{ IntegrationSuite }
type EdgeSuite struct{ IntegrationSuite }
type VersioningSuite struct{ IntegrationSuite }
type ConcurrencySuite struct{ IntegrationSuite }
type StressSuite struct{ IntegrationSuite }

func TestLifecycleIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(LifecycleSuite))
}

func TestDepsIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(DepsSuite))
}

func TestEdgeIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(EdgeSuite))
}

func TestVersioningIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(VersioningSuite))
}

func TestConcurrencyIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(ConcurrencySuite))
}

func TestStressIntegration(t *testing.T) {
    t.Parallel()
    suite.Run(t, new(StressSuite))
}
```

- [ ] **Step 3: Reassign test methods to the correct suite types**

Each test method must be attached to the appropriate suite type. Currently all methods are on `*IntegrationSuite`. Move them:

In `lifecycle_test.go` — change all `func (s *IntegrationSuite) TestLifecycle_*` to `func (s *LifecycleSuite) TestLifecycle_*`

In `deps_test.go` — change all `func (s *IntegrationSuite) TestDeps_*` to `func (s *DepsSuite) TestDeps_*`

In `edge_cases_test.go` — change all `func (s *IntegrationSuite) TestEdge_*` to `func (s *EdgeSuite) TestEdge_*`

In `versioning_test.go` — change all `func (s *IntegrationSuite) TestVersioning_*` to `func (s *VersioningSuite) TestVersioning_*`

In `concurrency_test.go` — change all `func (s *IntegrationSuite) TestConcurrency_*` to `func (s *ConcurrencySuite) TestConcurrency_*`

In `stress_test.go` — change all `func (s *IntegrationSuite) TestStress_*` to `func (s *StressSuite) TestStress_*`

- [ ] **Step 4: Update Makefile to use -parallel 6**

In `Makefile`, update the `test-integration` target:

```makefile
test-integration:
	@echo "$(BLUE)Running integration tests...$(NC)"
	@set -o pipefail; go test -tags integration -race -timeout 300s \
		-parallel 6 \
		./tests/integration/... -v 2>&1 | grep -v "malformed LC_DYSYMTAB"
	@echo "$(GREEN)Integration tests passed!$(NC)"
```

Note: timeout reduced from 600s to 300s — with parallel execution and short fixtures, 5 minutes is ample.

- [ ] **Step 5: Run the full integration suite locally**

Run: `make test-integration`

Expected: all tests PASS in under 2 minutes

- [ ] **Step 6: Commit**

```bash
git add tests/integration/env_test.go \
        tests/integration/suite_test.go \
        tests/integration/lifecycle_test.go \
        tests/integration/deps_test.go \
        tests/integration/edge_cases_test.go \
        tests/integration/versioning_test.go \
        tests/integration/concurrency_test.go \
        tests/integration/stress_test.go \
        Makefile
git commit -m "feat(integration): parallel test suites via homeDir injection — 5-6x speedup"
```

---

## Expected Outcome

| Metric | Before | After |
|---|---|---|
| CI wall time | 10+ min (timeout) | ~1.5 min |
| GORM SQL logs | Hundreds of 10KB blobs | None |
| service-b cleanup time | up to 60s | up to 5s |
| Test parallelism | 1 (sequential) | 6 suites concurrent |
| Timeout setting | 600s | 300s |

The SQL logging fix alone stops the I/O cascade that makes each test 20–30× slower than local. The parallel suites reduce the total by 5–6×. Combined, the test suite goes from timing out to completing in well under 2 minutes.
