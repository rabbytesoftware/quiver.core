# Integration Test Suite Design

> **For agentic workers:** Use `superpowers:writing-plans` to produce the implementation plan from this spec.

**Goal:** A blackbox integration test suite that stress-tests Quiver's full stack — manifest parsing, installation lifecycle, dependency graphs, updates, versioning, API surface, and error handling — exclusively through HTTP, with no access to internal Go types.

**Architecture:** One in-process go-git server hosts all fixture repos. Each test gets a fresh Quiver service and `httptest.Server` via `newEnv()`. Tests interact only through real HTTP requests and WebSocket connections. The entire suite runs under a single `//go:build integration` tag.

**Tech Stack:** `testify/suite`, `go-git` (in-process git server), `net/http/httptest`, `gorilla/websocket`, standard Go `net/http` client.

---

## Why This Suite Exists

Unit tests in `internal/` mock every boundary — vault, wizard, netbridge, manifold. They prove components work in isolation. This suite exists to catch the failures that only appear when everything runs together:

- State machine violations across the runner/installer/catalog boundary
- Dependency graph traversal bugs (deduplication, ordering, orphan cleanup)
- Concurrent state mutations that mocks can never reveal
- HTTP handler bugs: wrong status codes, malformed JSON, missing fields
- The `namespace@ref` identity model under versioning and coexistence scenarios

---

## Location and Package Structure

```
tests/
  integration/
    testdata/
      arrows/
        tool-a/            — simple tool, no deps, targets all 6 OS values
          arrow.yaml
        service-b/         — service with netbridge ports
          arrow.yaml
        composed-c/        — depends on tool-a + service-b
          arrow.yaml
        dep-chain/         — linear chain A→B→C→...→Z (26 arrows)
          a/arrow.yaml
          b/arrow.yaml
          ...
          z/arrow.yaml
        dep-diamond/       — diamond shape: root→left+right, left+right→shared
          root/arrow.yaml
          left/arrow.yaml
          right/arrow.yaml
          shared/arrow.yaml
        dep-wide/          — root with 15 direct dependencies
          root/arrow.yaml
          dep-01/arrow.yaml
          ...
          dep-15/arrow.yaml
        versioned/         — same arrow at two tags (v1, v2)
          v1/arrow.yaml
          v2/arrow.yaml
        malformed/         — syntactically broken YAML
          arrow.yaml
        invalid-ruleset/   — valid YAML, fails ruleset validation
          arrow.yaml
        no-current-os/     — targets only OSes other than the test runner's
          arrow.yaml
        dep-circular/      — circ-a depends on circ-b, circ-b depends on circ-a
          circ-a/arrow.yaml
          circ-b/arrow.yaml
        missing-vars/      — variables declared with no defaults
          arrow.yaml
    suite_test.go          — IntegrationSuite, SetupSuite/TearDownSuite
    env_test.go            — Env struct, newEnv()
    git_test.go            — in-process go-git HTTP server
    client_test.go         — HTTP + WebSocket helpers
    fixtures_test.go       — fixture file loaders
    lifecycle_test.go      — full arrow lifecycle scenarios
    deps_test.go           — dependency graph scenarios
    versioning_test.go     — versioning and coexistence scenarios
    concurrency_test.go    — concurrent operation scenarios
    edge_cases_test.go     — state machine violations + manifest edge cases
    stress_test.go         — large graphs, bulk operations, restart survival
```

All files are `package integration_test`. All test files carry `//go:build integration`. Helper files (`env_test.go`, `git_test.go`, `client_test.go`, `fixtures_test.go`) are `_test.go` files — they compile into the same test binary and share their symbols with all scenario files without any import gymnastics.

---

## Infrastructure

### In-Process Git Server (`git_test.go`)

Started once in `SetupSuite`. Serves every directory under `testdata/arrows/` as a separate bare git repo over HTTP, each with the fixture's `arrow.yaml` committed under a `v1` tag (or both `v1` and `v2` for the `versioned/` fixture).

Each fixture directory is a separate bare repo. Nested paths (e.g. `dep-diamond/root`) are served as `http://gitserver/dep-diamond/root` and map to the namespace `dep-diamond/root@v1`. The git server must support path-rooted repo URLs, not just flat names.

```go
type gitServer struct {
    URL string
    // internal go-git repos
}

func startGitServer(t *testing.T) *gitServer
```

The server uses `go-git`'s in-process HTTP transport — no external `git` binary required.

### Test Environment (`env_test.go`)

Each test calls `newEnv()`. It creates:

1. A `t.TempDir()` as the isolated `QUIVER_HOME`
2. A fully wired `ArrowService` via `arrow.NewArrowBuilder()`, pointed at the git server's URL as the manifold resolver base and at the temp dir for all storage
3. An `httptest.NewServer` wrapping the real Gin router (same wiring as the production daemon)

```go
type Env struct {
    URL string   // base URL for HTTP requests
    // internal handles for cleanup
}

func (s *IntegrationSuite) newEnv() *Env
```

`newEnv()` registers a `t.Cleanup` that shuts down the `httptest.Server` and closes all stores. Tests never call teardown manually.

### HTTP Client (`client_test.go`)

Thin helpers over `net/http`. No mocking.

```go
func (e *Env) Add(ns string) *http.Response
func (e *Env) Remove(ns string) *http.Response
func (e *Env) Install(ns string, vars map[string]string) *http.Response
func (e *Env) Uninstall(ns string, vars map[string]string) *http.Response
func (e *Env) Execute(ns, method string, vars map[string]string) *http.Response
func (e *Env) Stop(ns string) *http.Response
func (e *Env) Update(ns string, opts UpdateRequest) *http.Response
func (e *Env) List() *http.Response
func (e *Env) GetDetail(ns string) *http.Response
func (e *Env) Seed(ns string, body []byte) *http.Response
func (e *Env) Validate(ns string, body []byte) *http.Response
func (e *Env) DialRuntime(ns string) *wsConn  // WebSocket
```

All helpers return the raw `*http.Response`. Assertions are in the test, not the helper.

### Suite (`suite_test.go`)

```go
type IntegrationSuite struct {
    suite.Suite
    git *gitServer
}

func (s *IntegrationSuite) SetupSuite()    // starts git server once
func (s *IntegrationSuite) TearDownSuite() // stops git server

func TestIntegration(t *testing.T) {
    suite.Run(t, new(IntegrationSuite))
}
```

---

## Test Scenarios

### `lifecycle_test.go` — Arrow Lifecycle

**Full round-trip**
`POST /arrow/tool-a@v1` → 201. `GET /arrow/tool-a@v1` → state absent. Install → state ready. Execute `_execute` → state executing → ready. Stop → stopping → ready. Uninstall → uninstalling → removed from catalog. `DELETE /arrow/tool-a@v1` → 204. `GET /arrow/tool-a@v1` → 404.

**Add idempotency**
`POST /arrow/tool-a@v1` twice → second call returns 409 Conflict.

**Install idempotency**
Install an already-installed arrow → returns a clear error (not a panic, not corrupt state).

**Uninstall cleans vault**
After uninstall succeeds, the vault entry for the arrow is gone. Re-adding and re-installing starts clean.

**State streams via WebSocket**
Connect `GET /arrow.runtime/tool-a@v1` before install. Drive the install. Assert the WebSocket emits state transitions in order: `installing` → `ready`.

**Execute unknown method**
`POST /arrow/tool-a@v1/_unknownmethod` → 4xx. State unchanged.

---

### `deps_test.go` — Dependency Management

**Transitive install**
Add `composed-c@v1` (depends on `tool-a@v1` and `service-b@v1`). Install `composed-c@v1`. Assert `tool-a@v1` and `service-b@v1` appear in the catalog and reach state `ready` before `composed-c@v1`.

**Diamond deduplication**
Fixture: `dep-diamond/root` depends on `left` and `right`; both depend on `shared`. Install `dep-diamond/root@v1`. Assert `shared@v1` is installed exactly once (check catalog, not install count).

**Circular dependency detection**
Fixture: `dep-circular/circ-a` depends on `circ-b` which depends on `circ-a`. `POST /arrow/dep-circular/circ-a@v1` → returns an error. Neither arrow appears in the catalog.

**Remove blocked by dependents**
`composed-c@v1` is installed. `DELETE /arrow/tool-a@v1` → 409 (dependents exist). `tool-a@v1` is still in the catalog.

**Remove after dependents removed**
Uninstall and remove `composed-c@v1`. Then `DELETE /arrow/tool-a@v1` → 204.

**Orphan cleanup after uninstall**
Install `composed-c@v1` (auto-installs `tool-a@v1`, `service-b@v1`). Uninstall `composed-c@v1` with `uninstall_orphans: true`. Assert `tool-a@v1` and `service-b@v1` are also uninstalled.

---

### `versioning_test.go` — Versioning and Coexistence

**Two versions coexist**
Add `versioned@v1` and `versioned@v2`. Both appear in `GET /arrow` under the same bare namespace with two version entries. Installing one does not affect the other's state.

**Version pin survives update**
Add `versioned@v1`. `PATCH /arrow/versioned@v1` with `upgrade_ref: false`. Arrow stays at v1; no manifest change from v2.

**Upgrade to newer ref**
Add `versioned@v1`. `PATCH /arrow/versioned@v1` with `upgrade_ref: true`. Assert the arrow is now at v2, vault holds the v2 manifest, and v1 is removed from the catalog.

**Constraint resolution**
Add `versioned@~v1` (semver glob). Assert the resolved ref is `v1` (the latest matching tag). Re-add after a v2 tag exists on the same repo. Assert constraint now resolves to `v2`.

**Dep added in v2**
`versioned@v2` fixture declares a new dependency not in v1. Upgrade from v1 to v2 with `install_added: true`. Assert the new dep is installed.

**Dep removed in v2**
`versioned@v2` fixture drops a dep that was in v1. Upgrade with `uninstall_orphans: true`. Assert the dropped dep is uninstalled.

---

### `concurrency_test.go` — Concurrent Operations

**Concurrent Add same namespace**
Fire 10 goroutines all calling `POST /arrow/tool-a@v1` simultaneously. Assert exactly one succeeds (201) and the rest return 409. Assert exactly one catalog entry exists.

**Concurrent installs shared dep**
Fire `Install(composed-c@v1)` and `Install(another-arrow-that-uses-tool-a@v1)` simultaneously. Assert `tool-a@v1` ends in state `ready`, not stuck or double-installed.

**Concurrent install + uninstall**
Start install of `tool-a@v1`. Immediately fire uninstall. Assert the final state is consistent — either fully installed or fully removed. No stuck `installing` state.

**Concurrent list under load**
Fire 50 concurrent `GET /arrow` requests while 5 installs are in flight. Assert all list responses are valid JSON (no partial writes, no panics).

---

### `edge_cases_test.go` — State Machine and Manifest Edge Cases

**Execute while installing**
Start a long install (fixture with a slow step). Immediately fire `POST /arrow/tool-a@v1/_execute`. Assert 409 or appropriate rejection. Install completes normally.

**Install while already installing**
Trigger install. While it is in the `installing` state, trigger install again. Second call returns an error. Only one install proceeds.

**Update while running**
Start a `service-b@v1` execute (long-running service). Fire `PATCH /arrow/service-b@v1`. Assert update is rejected while running.

**Remove while installing**
Start install. Fire `DELETE /arrow/tool-a@v1`. Assert rejection. Install completes; arrow remains in catalog.

**Context cancellation mid-install**
Start install with a short deadline context. Cancel it. Assert arrow is not left in `installing` state — it transitions to an error/absent state cleanly.

**Malformed YAML**
`SEED /arrow/bad@v1` with the `malformed/arrow.yaml` fixture. Assert 400. No catalog entry created.

**Ruleset violation**
`SEED /arrow/bad@v1` with `invalid-ruleset/arrow.yaml`. Assert 422 with a `ValidationError` list in the response body.

**No target for current OS**
`POST /arrow/no-current-os@v1`. Assert the add fails (or succeeds with a warning) and install is rejected with a clear "no target for current OS" error.

**Variables with no defaults**
`POST /arrow/missing-vars@v1`. Install without providing vars. Assert install is rejected with a variable resolution error.

**Name at MaxNameLength (255 chars)**
`SEED` a manifest with a 255-character name. Assert 200 and the name round-trips correctly in `GET /arrow/:ns`.

**Name exceeding MaxNameLength**
`SEED` a manifest with a 256-character name. Assert 422 with a ruleset error on the `name` field.

---

### `stress_test.go` — Large Graphs and Bulk Operations

**Deep chain: 26 levels**
Add `dep-chain/a` (which transitively depends A→B→C→...→Z). Install `a`. Assert all 26 arrows reach state `ready` in topological order (Z installed before Y, Y before X, etc.).

**Wide graph: 15 direct deps**
Add `dep-wide/root` (depends on 15 arrows). Install `root`. Assert all 15 deps are installed before root reaches `ready`.

**Bulk add 100 arrows**
Add 100 distinct `tool-a@v1`-clones (same fixture, different namespaces). `GET /arrow` returns all 100. No timeouts, no panics.

**Service restart survives**
Install `tool-a@v1`. Destroy the `httptest.Server` and create a new `newEnv()` pointed at the same `QUIVER_HOME`. `GET /arrow/tool-a@v1` returns the arrow with state `ready`. State survived the restart.

**List performance under volume**
With 100 arrows in the catalog, `GET /arrow` responds within 500ms.

---

## Fixture Arrow Manifests

All fixture manifests are valid `arrow.yaml` v0 files. Each has lifecycle steps that echo a marker string to stdout so tests can assert on wizard output when needed.

`tool-a` is the minimal fixture — single lifecycle step per phase, no deps, targets all 6 OS values (`linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`, `darwin/amd64`, `darwin/arm64`). All other fixtures extend this pattern.

`service-b` adds a `netbridge` port declaration to exercise the network allocation path.

`composed-c` declares `tool-a` and `service-b` as `tools` and `services` dependencies respectively.

`dep-chain/*` and `dep-wide/*` use the same minimal lifecycle as `tool-a`; their interest is purely in the dependency graph shape.

`versioned/v1` and `versioned/v2` are identical except `v2` bumps the version field, changes one lifecycle step body, adds one dependency, and removes another. This enables the full update/upgrade test matrix.

`malformed/arrow.yaml` is not valid YAML (intentional syntax error).

`invalid-ruleset/arrow.yaml` is valid YAML with a missing required lifecycle field.

`no-current-os/arrow.yaml` declares targets only for a platform that is never the test runner's OS.

`missing-vars/arrow.yaml` declares one variable with no default and no value in the environment.

---

## Makefile Integration

```makefile
test-integration:
    @echo "$(BLUE)Running integration tests...$(NC)"
    @set -o pipefail; go test -tags integration -race -timeout 300s \
        ./tests/integration/... -v 2>&1 | grep -v "malformed LC_DYSYMTAB"
    @echo "$(GREEN)Integration tests passed!$(NC)"

test-all: test test-integration
    @echo "$(GREEN)All tests passed!$(NC)"

pr-checks-full: pr-checks test-integration
    @echo "$(GREEN)Full PR checks passed!$(NC)"
```

`test-integration` runs the full suite — there is no partial mode. The git server is always live; every test exercises the resolver. Timeout is 300 seconds to accommodate the deep-chain and bulk stress tests.

`test-all` gates on both unit and integration. `pr-checks-full` is the CI gate for release branches.

---

## What This Suite Does Not Cover

- **Windows shell execution:** `run` steps that invoke `.bat` scripts are not tested here. The `wizard/step/run` handler has its own unit tests for Windows. Integration tests assume a Unix test runner.
- **UPnP/NAT-PMP discovery:** The netbridge port allocation strategy is tested in `netbridge_test.go` with injected strategies. Integration tests use the default strategy which falls back to local port allocation.
- **Quiver manifests:** This suite covers `arrow.yaml` only. Quiver manifest lifecycle has its own service and is out of scope here.
- **Authentication/authorization:** Not yet implemented in Quiver. No auth tests here.
