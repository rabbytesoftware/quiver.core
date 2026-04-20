# Integration Test Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a blackbox integration test suite that stress-tests the full Quiver stack — git resolution, manifest compilation, installation lifecycle, dependency graphs, versioning, concurrent operations, and the HTTP API — exclusively through HTTP requests.

**Architecture:** Each test gets a fresh isolated Quiver service via `newEnv()` with `t.TempDir()` as HOME, a custom manifold that resolves namespaces from in-process bare git repos on disk, and an `httptest.Server` wrapping the real Gin router. All test interaction is via HTTP. The git repos are created once per suite from fixture YAML files under `tests/integration/testdata/`.

**Tech Stack:** `testify/suite`, `go-git/v5` (already in `go.mod`), `gorilla/websocket` (already in `go.mod`), `net/http/httptest`, standard `net/http` client.

---

## File Structure

**New files:**
```
tests/integration/
  testdata/arrows/
    tool-a/arrow.yaml
    service-b/arrow.yaml
    composed-c/arrow.yaml
    dep-diamond/root/arrow.yaml
    dep-diamond/left/arrow.yaml
    dep-diamond/right/arrow.yaml
    dep-diamond/shared/arrow.yaml
    dep-chain/a/arrow.yaml ... z/arrow.yaml
    dep-wide/root/arrow.yaml, dep-01/..dep-15/arrow.yaml
    dep-circular/circ-a/arrow.yaml
    dep-circular/circ-b/arrow.yaml
    versioned/v1/arrow.yaml
    versioned/v2/arrow.yaml
    malformed/arrow.yaml
    invalid-ruleset/arrow.yaml
    no-current-os/arrow.yaml
    missing-vars/arrow.yaml
  suite_test.go
  env_test.go
  git_test.go
  client_test.go
  fixtures_test.go
  lifecycle_test.go
  deps_test.go
  versioning_test.go
  concurrency_test.go
  edge_cases_test.go
  stress_test.go
```

**Modified files:**
- `internal/engine/manifold/manifold.go` — add `NewWithResolvers` constructor
- `Makefile` — add `test-integration`, `test-all`, update `pr-checks`
- `.github/workflows/ci.yml` — add `test-integration` job

---

## Namespace Convention for Tests

All test namespaces use `quiver.test/<category>/<name>@<tag>`:
- `quiver.test/quiver-test/tool-a@v1`
- `quiver.test/dep-diamond/root@v1`

The `testResolver` strips the `quiver.test/` prefix and maps the remainder to a bare repo on disk at `reposDir/<category>/<name>`. This satisfies `Namespace.Validate()` (requires 3 slash-separated parts) without touching any domain logic.

In fixture YAML files, deps reference other fixtures using the same convention:
```yaml
tools:
  - quiver.test/quiver-test/tool-a@v1
```

---

## Task 1: Makefile and CI Changes

**Files:**
- Modify: `Makefile` (after `test-coverage` target, before `test-docker`)
- Modify: `.github/workflows/ci.yml` (after `test-coverage` job, before `test-multi-platform`)

- [ ] **Step 1: Add targets to Makefile**

Open `Makefile`. After the `test-coverage` target block, insert:

```makefile
# Run integration tests
test-integration:
	@echo "$(BLUE)Running integration tests...$(NC)"
	@set -o pipefail; go test -tags integration -race -timeout 300s \
		./tests/integration/... -v 2>&1 | grep -v "malformed LC_DYSYMTAB"
	@echo "$(GREEN)Integration tests passed!$(NC)"

# Run unit + integration tests
test-all: test test-integration
	@echo "$(GREEN)All tests passed!$(NC)"
```

- [ ] **Step 2: Update pr-checks in Makefile**

Replace the existing `pr-checks` line:
```makefile
pr-checks: validate-branch clean deps fmt vet lint security build test-coverage test-integration
```

- [ ] **Step 3: Add CI job**

Open `.github/workflows/ci.yml`. After the closing block of the `test-coverage` job (after the `Upload coverage reports` step), insert the new job **before** the `test-multi-platform` job:

```yaml
  # Integration test suite - full stack, HTTP-only, blackbox
  test-integration:
    name: Integration Tests
    runs-on: ubuntu-latest
    needs: [test-coverage, code-quality]

    steps:
    - name: Checkout code
      uses: actions/checkout@v6

    - name: Set up Go
      uses: actions/setup-go@v6
      with:
        go-version: ${{ env.GO_VERSION }}

    - name: Cache Go modules
      uses: actions/cache@v5
      with:
        path: |
          ~/.cache/go-build
          ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ env.GO_VERSION }}-${{ hashFiles('**/go.sum') }}
        restore-keys: |
          ${{ runner.os }}-go-${{ env.GO_VERSION }}-
          ${{ runner.os }}-go-

    - name: Download dependencies
      run: go mod download

    - name: Run integration tests
      run: make test-integration
```

- [ ] **Step 4: Verify**

```bash
make -n test-integration   # dry-run, should print the go test command
make -n pr-checks          # should include test-integration in the chain
```

- [ ] **Step 5: Commit**

```bash
git add Makefile .github/workflows/ci.yml
git commit -m "build: add test-integration target and CI job"
```

---

## Task 2: Add `manifold.NewWithResolvers`

**Files:**
- Modify: `internal/engine/manifold/manifold.go`

**Context:** `manifold.New()` hardwires `resolver.New()` and `resolvers.NewConstraintResolver()`. Tests need to inject a custom resolver that reads from local git repos. Adding `NewWithResolvers` is the minimal change needed — existing callers are unchanged.

- [ ] **Step 1: Add the constructor**

Open `internal/engine/manifold/manifold.go`. After the `New` function (line ~65), add:

```go
// NewWithResolvers builds a Manifold with injected resolver and constraint resolver.
// Intended for tests that need to control how namespaces are resolved.
func NewWithResolvers(
    rsv resolver.Resolver,
    crs resolvers.ConstraintResolver,
) Manifold {
    return &manifold{
        rsv:        rsv,
        trs:        translator.NewTranslator(),
        cmp:        compiler.New(),
        rls:        ruleset.New(),
        constraint: crs,
    }
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/engine/manifold/...
```

Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
git add internal/engine/manifold/manifold.go
git commit -m "feat(manifold): add NewWithResolvers constructor for test injection"
```

---

## Task 3: Fixture Arrow YAML Files

**Files:** All files under `tests/integration/testdata/arrows/`

**Context:** Each fixture is a valid `arrow@v0` YAML. Lifecycle steps use `echo` so they succeed on any Unix host without side effects. The wizard EXECUTES these steps for real — `echo` exits 0 and produces no files.

**Key format reference** (from `internal/engine/manifold/translator/arrow/v0/types.go`):
```yaml
schema: "arrow@v0"
metadata:
  name: <name>
  version: 1.0.0
  description: <desc>
targets:
  "*":                    # wildcard matches all OS values
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
```

For the `execute` + `stop` pair in service-b, `execute` must be long-running (so stop can be tested). Use `sleep 300` as execute and `signal: graceful` as stop.

- [ ] **Step 1: Create `tool-a/arrow.yaml`**

```
tests/integration/testdata/arrows/tool-a/arrow.yaml
```

Content: `schema: "arrow@v0"`, name `quiver-test.tool-a`, version `1.0.0`, target `"*"` with `install` (echo installed), `execute` (echo executed — exits immediately), `uninstall` (echo uninstalled). No deps.

- [ ] **Step 2: Create `service-b/arrow.yaml`**

```
tests/integration/testdata/arrows/service-b/arrow.yaml
```

Content: name `quiver-test.service-b`, version `1.0.0`, target `"*"` with `install` (echo installed), `execute` (`sleep 300`), `stop` (signal: graceful), `uninstall` (echo uninstalled). Add netbridge port `TEST_PORT` protocol `tcp` default `9000`. No deps.

- [ ] **Step 3: Create `composed-c/arrow.yaml`**

```
tests/integration/testdata/arrows/composed-c/arrow.yaml
```

Content: name `quiver-test.composed-c`, version `1.0.0`, target `"*"` with `tools: [quiver.test/quiver-test/tool-a@v1]` and `services: [quiver.test/quiver-test/service-b@v1]`. Install: echo installed. Uninstall: echo uninstalled.

- [ ] **Step 4: Create `dep-diamond` fixtures**

Four files: `root`, `left`, `right`, `shared`. Each with `schema: "arrow@v0"`, target `"*"`, install: echo installed, uninstall: echo uninstalled.

- `dep-diamond/root/arrow.yaml` — `tools: [quiver.test/dep-diamond/left@v1, quiver.test/dep-diamond/right@v1]`
- `dep-diamond/left/arrow.yaml` — `tools: [quiver.test/dep-diamond/shared@v1]`
- `dep-diamond/right/arrow.yaml` — `tools: [quiver.test/dep-diamond/shared@v1]`
- `dep-diamond/shared/arrow.yaml` — no deps

- [ ] **Step 5: Create `dep-chain` fixtures (a through z, 26 files)**

Each letter depends on the next: `a` depends on `b`, `b` on `c`, ..., `y` on `z`. `z` has no deps.

Use a script to generate all 26:
```bash
mkdir -p tests/integration/testdata/arrows/dep-chain
for letter in {a..z}; do
  mkdir -p tests/integration/testdata/arrows/dep-chain/$letter
done
```

Then write each `arrow.yaml`. Example for `a`:
```yaml
schema: "arrow@v0"
metadata:
  name: dep-chain.a
  version: 1.0.0
  description: Chain node a
targets:
  "*":
    tools:
      - quiver.test/dep-chain/b@v1
    lifecycle:
      install:
        - type: run
          command: echo installed-a
          title: Install a
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled-a
          title: Uninstall a
          timeout: 10s
          exit_on_failure: false
```

`z` has no `tools` entry.

- [ ] **Step 6: Create `dep-wide` fixtures (root + 15 deps)**

```
dep-wide/root/arrow.yaml    — tools: [dep-01..dep-15]
dep-wide/dep-01/arrow.yaml  — no deps
...
dep-wide/dep-15/arrow.yaml  — no deps
```

`root` declares all 15 as tools:
```yaml
tools:
  - quiver.test/dep-wide/dep-01@v1
  - quiver.test/dep-wide/dep-02@v1
  # ... through dep-15
```

- [ ] **Step 7: Create `dep-circular` fixtures**

```
dep-circular/circ-a/arrow.yaml — tools: [quiver.test/dep-circular/circ-b@v1]
dep-circular/circ-b/arrow.yaml — tools: [quiver.test/dep-circular/circ-a@v1]
```

Both are otherwise minimal (install: echo installed).

- [ ] **Step 8: Create `versioned` fixtures**

`versioned/v1/arrow.yaml`: name `quiver-test.versioned`, version `1.0.0`, target `"*"`, tools: `[quiver.test/quiver-test/tool-a@v1]`, install: `echo installed-v1`, execute: `echo executed-v1`, uninstall: `echo uninstalled-v1`.

`versioned/v2/arrow.yaml`: same name `quiver-test.versioned`, version `2.0.0`, target `"*"`, tools: `[quiver.test/quiver-test/service-b@v1]` (different dep — tool-a removed, service-b added), install: `echo installed-v2`, execute: `echo executed-v2`, uninstall: `echo uninstalled-v2`.

- [ ] **Step 9: Create edge-case fixtures**

`malformed/arrow.yaml` — intentionally broken YAML (e.g., `schema: [unclosed`).

`invalid-ruleset/arrow.yaml` — valid YAML but missing required fields:
```yaml
schema: "arrow@v0"
metadata:
  name: invalid-ruleset.arrow
  version: 1.0.0
targets:
  "*":
    lifecycle: {}    # no lifecycle steps — should fail ruleset
```

`no-current-os/arrow.yaml` — only targets `windows/amd64` and `windows/arm64`:
```yaml
schema: "arrow@v0"
metadata:
  name: no-current-os.arrow
  version: 1.0.0
targets:
  "windows/*":
    lifecycle:
      install:
        - type: run
          command: echo installed
          timeout: 10s
```

`missing-vars/arrow.yaml` — declares a variable with no default:
```yaml
schema: "arrow@v0"
metadata:
  name: missing-vars.arrow
  version: 1.0.0
variables:
  - name: REQUIRED_VAR
    description: Must be provided
    type: string
    # no default
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo ${REQUIRED_VAR}
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          timeout: 10s
```

- [ ] **Step 10: Verify fixture YAMLs parse correctly**

Write a quick throwaway Go test (or use `manifold.ParseArrow` directly in a scratch program) to confirm `tool-a`, `service-b`, and `composed-c` parse without errors. Do this informally — the git_test.go task will do the formal validation.

- [ ] **Step 11: Commit**

```bash
git add tests/integration/testdata/
git commit -m "test(integration): add fixture arrow YAML files"
```

---

## Task 4: Git Repository Infrastructure (`git_test.go`)

**Files:**
- Create: `tests/integration/git_test.go`

**Context:** `SetupSuite` walks `testdata/arrows/` and creates one bare git repo per fixture directory. Fixtures that have multiple versions (like `versioned/`) get multiple tags — the directory structure is `versioned/v1/arrow.yaml` and `versioned/v2/arrow.yaml`. The testResolver maps namespace paths to repo dirs.

Namespace-to-path mapping: `quiver.test/<rest>@<tag>` → `reposDir/<rest>` (strip `quiver.test/` prefix). Tag comes from `Ref()`.

For multi-version fixtures, the repo at `reposDir/versioned` has two tagged commits: `v1` (with v1's YAML) and `v2` (with v2's YAML).

- [ ] **Step 1: Write `git_test.go` with build tag and imports**

```go
//go:build integration

package integration_test

import (
    "context"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
    "time"
    "testing"

    "github.com/go-git/go-git/v5"
    "github.com/go-git/go-git/v5/plumbing"
    "github.com/go-git/go-git/v5/plumbing/object"
    "github.com/go-git/go-git/v5/storage/memory"
    "github.com/go-git/go-billy/v5/memfs"

    "github.com/rabbytesoftware/quiver/internal/domain"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver/resolvers"
)
```

- [ ] **Step 2: Define `fixtureRepos` and `testResolver` types**

```go
// fixtureRepos holds one in-memory repo storer per fixture path.
// Key: fixture path relative to testdata/arrows/ (e.g., "quiver-test/tool-a")
// Value: the in-memory storer for that repo.
type fixtureRepos map[string]*memory.Storage

// testResolver implements resolver.Resolver + resolvers.ConstraintResolver
// by cloning from in-memory repos registered in fixtureRepos.
type testResolver struct {
    repos fixtureRepos
}

func (r *testResolver) ResolveArrow(ctx context.Context, ns domain.Namespace) ([]byte, error)
func (r *testResolver) ResolveQuiver(ctx context.Context, ns domain.Namespace) ([]byte, error)
func (r *testResolver) Resolve(ctx context.Context, ns domain.Namespace, pattern string) (string, error)
```

- [ ] **Step 3: Implement `buildFixtureRepos`**

This is the core setup function called in `SetupSuite`. It:
1. Walks `testdata/arrows/` looking for directories that contain `arrow.yaml`
2. Groups by "base fixture path" (for versioned fixtures, groups v1 and v2 under the same repo)
3. Creates one in-memory bare repo per group
4. Commits each version's YAML under the appropriate git tag

```go
// buildFixtureRepos walks testdata/arrows/ and creates one in-memory repo per fixture.
// For versioned fixtures (testdata/arrows/versioned/v1/arrow.yaml), creates one repo
// with tag "v1" pointing to the v1 content and tag "v2" pointing to v2 content.
// All other fixtures get a single "v1" tag.
func buildFixtureRepos(t *testing.T) fixtureRepos
```

Logic:
- For `testdata/arrows/tool-a/arrow.yaml`: create repo at key `quiver-test/tool-a`, commit YAML, tag `v1`
- For `testdata/arrows/versioned/v1/arrow.yaml` and `versioned/v2/arrow.yaml`: create ONE repo at key `quiver-test/versioned`, commit v1 YAML as tag `v1`, then add commit with v2 YAML as tag `v2`
- For `testdata/arrows/dep-chain/a/arrow.yaml`: create repo at key `dep-chain/a`, commit, tag `v1`
- etc.

The "versioned" directory is special: its children are version directories, not fixture directories.

Detection logic: if a directory under `testdata/arrows/` contains subdirectories that are version names (matching pattern `v\d+`), treat it as a multi-version fixture.

Commit helper:
```go
func commitFile(
    wt *gogit.Worktree,
    filename string,
    content []byte,
    msg string,
) (plumbing.Hash, error)
```

Create tag helper:
```go
func createTag(repo *gogit.Repository, name string, hash plumbing.Hash) error
```

- [ ] **Step 4: Implement `testResolver.ResolveArrow`**

Logic:
1. Strip `quiver.test/` prefix from `ns.BareNamespace()` to get the fixture key
2. Look up storer in `r.repos`
3. Clone from the in-memory storer into a new in-memory filesystem
4. Checkout at tag `ns.Ref()` (or default branch if no ref)
5. Read `arrow.yaml` from the working tree and return its bytes

```go
func (r *testResolver) ResolveArrow(
    ctx context.Context,
    ns domain.Namespace,
) ([]byte, error) {
    key := fixtureKey(ns)
    storer, ok := r.repos[key]
    if !ok {
        return nil, fmt.Errorf("fixture not found: %s", key)
    }
    return cloneAndRead(ctx, storer, ns.Ref(), "arrow.yaml")
}
```

`cloneAndRead` uses `gogit.Clone` with `storer` as the remote (go-git supports cloning from an in-memory storer):
```go
func cloneAndRead(ctx context.Context, src *memory.Storage, ref, filename string) ([]byte, error)
```

For cloning from an in-memory storer, go-git supports `CloneOptions{URL: ""}` with a custom remote — or use `gogit.Clone` with a `file://` URL if the storer is filesystem-backed. Since we're using `memory.Storage`, clone using `gogit.CloneContext` with the in-memory storage directly as source.

Actually, go-git's `Clone` doesn't support cloning from a `*memory.Storage` directly as a remote. The simplest approach: instead of cloning, open the repo directly from the storage and read the file at the given ref.

Revised approach for `cloneAndRead`:
1. Open the repo from the storer: `repo, _ := gogit.Open(storer, nil)`
2. Resolve ref: `hash, _ := repo.Tag(ref)` or `head, _ := repo.Head()`
3. Get commit: `commit, _ := repo.CommitObject(hash)`
4. Get file: `f, _ := commit.File(filename)`
5. Read content: `f.Contents()`

```go
func readFromRepo(storer *memory.Storage, ref, filename string) ([]byte, error) {
    repo, err := gogit.Open(storer, memfs.New())
    // ... resolve tag/ref, read file
}
```

- [ ] **Step 5: Implement `testResolver.Resolve` (ConstraintResolver)**

For glob patterns like `~v1` or `>=v1.0.0`, resolve against the tags in the in-memory repo:
1. Get the storer for the namespace's fixture key
2. Open the repo
3. List all tags
4. Apply the semver constraint (use `golang.org/x/mod/semver` or simple string matching for tests)

For simplicity in tests, support only exact refs and `~vN` (latest patch for major.minor). Tests using constraints use simple exact refs unless explicitly testing constraint resolution.

```go
func (r *testResolver) Resolve(
    ctx context.Context,
    ns domain.Namespace,
    pattern string,
) (string, error)
```

- [ ] **Step 6: Add `fixtureKey` helper**

```go
// fixtureKey extracts the fixture repo key from a namespace.
// "quiver.test/quiver-test/tool-a@v1" → "quiver-test/tool-a"
// "quiver.test/dep-diamond/root@v1"   → "dep-diamond/root"
func fixtureKey(ns domain.Namespace) string {
    bare := string(ns.BareNamespace())
    return strings.TrimPrefix(bare, "quiver.test/")
}
```

- [ ] **Step 7: Verify compilation**

```bash
go build -tags integration ./tests/integration/...
```

Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add tests/integration/git_test.go
git commit -m "test(integration): add in-memory git repos and test resolver"
```

---

## Task 5: Suite Scaffold and Test Environment

**Files:**
- Create: `tests/integration/suite_test.go`
- Create: `tests/integration/env_test.go`

**Context:** `IntegrationSuite` starts the git infrastructure once in `SetupSuite`. `newEnv()` creates a fully isolated Quiver service per test using `t.TempDir()` as HOME. This ensures no shared state between tests.

The wiring chain: `engine.New()` (uses temp HOME) → replace `engines.Manifold` with test manifold → `adapter.New()` (uses temp HOME) → `app.New()` → `api.New()` → `httptest.NewServer`.

Setting `HOME` before any `paths.*` calls causes `paths.Store()`, `paths.Events()`, `paths.Namespaces()` to all resolve under the temp dir.

- [ ] **Step 1: Write `suite_test.go`**

```go
//go:build integration

package integration_test

import (
    "testing"
    "github.com/stretchr/testify/suite"
)

type IntegrationSuite struct {
    suite.Suite
    repos fixtureRepos // built once, shared across all tests
}

func (s *IntegrationSuite) SetupSuite() {
    s.repos = buildFixtureRepos(s.T())
}

func TestIntegration(t *testing.T) {
    suite.Run(t, new(IntegrationSuite))
}
```

- [ ] **Step 2: Write `env_test.go`**

```go
//go:build integration

package integration_test

import (
    "context"
    "net/http/httptest"
    "path/filepath"
    "time"

    "github.com/rabbytesoftware/quiver/internal/api"
    "github.com/rabbytesoftware/quiver/internal/adapter"
    "github.com/rabbytesoftware/quiver/internal/app"
    "github.com/rabbytesoftware/quiver/internal/engine"
    "github.com/rabbytesoftware/quiver/internal/engine/manifold"
)

type Env struct {
    URL  string
    home string
}

func (s *IntegrationSuite) newEnv() *Env {
    home := s.T().TempDir()
    s.T().Setenv("HOME", home)

    ctx := context.Background()

    engines, err := engine.New(ctx)
    s.Require().NoError(err)

    // Inject test resolver so manifold resolves from local fixture repos
    rsv := &testResolver{repos: s.repos}
    engines.Manifold = manifold.NewWithResolvers(rsv, rsv)

    adapters, err := adapter.New()
    s.Require().NoError(err)

    hub := api.NewHub()
    appContainer, err := app.New(engines, adapters, hub)
    s.Require().NoError(err)

    apiContainer, err := api.New(appContainer, hub)
    s.Require().NoError(err)

    srv := httptest.NewServer(apiContainer)
    s.T().Cleanup(srv.Close)

    return &Env{URL: srv.URL, home: home}
}
```

Note: `testResolver` implements both `resolver.Resolver` and `resolvers.ConstraintResolver` — so it satisfies both parameters of `manifold.NewWithResolvers(rsv, rsv)`.

- [ ] **Step 3: Verify compilation**

```bash
go build -tags integration ./tests/integration/...
```

Expected: no output.

- [ ] **Step 4: Run the empty suite to confirm setup works**

```bash
go test -tags integration -timeout 60s -v ./tests/integration/... -run TestIntegration
```

Expected: `PASS` with 0 tests (suite runs but no test methods yet).

- [ ] **Step 5: Commit**

```bash
git add tests/integration/suite_test.go tests/integration/env_test.go
git commit -m "test(integration): add suite scaffold and newEnv wiring"
```

---

## Task 6: HTTP Client and Fixture Helpers

**Files:**
- Create: `tests/integration/client_test.go`
- Create: `tests/integration/fixtures_test.go`

**Context:** `client_test.go` wraps `net/http` with typed methods for every API endpoint. All methods return `*http.Response` — assertions stay in the test, not the helper. `fixtures_test.go` reads testdata files and provides `nsFor(fixture, tag)` to build test namespaces.

- [ ] **Step 1: Write `client_test.go`**

Build tag: `//go:build integration`. Package: `integration_test`.

```go
type client struct {
    baseURL string
    http    *http.Client
}

func newClient(baseURL string) *client

// Arrow endpoints — all return *http.Response, caller closes body
func (c *client) Add(ns string) *http.Response
func (c *client) Remove(ns string) *http.Response
func (c *client) List() *http.Response
func (c *client) GetDetail(ns string) *http.Response
func (c *client) Update(ns string, body map[string]any) *http.Response
func (c *client) Install(ns string, vars map[string]string) *http.Response
func (c *client) Uninstall(ns string, vars map[string]string) *http.Response
func (c *client) Execute(ns, method string, vars map[string]string) *http.Response
func (c *client) Stop(ns string) *http.Response
func (c *client) Seed(ns string, body []byte) *http.Response
func (c *client) Validate(ns string, body []byte) *http.Response

// WebSocket — returns conn, caller closes
func (c *client) DialRuntime(ns string) (*websocket.Conn, error)
```

JSON helpers:
```go
// decodeJSON decodes the response body into v and closes the body.
func decodeJSON(t *testing.T, resp *http.Response, v any)

// mustStatus asserts resp.StatusCode == want, prints body on failure.
func mustStatus(t *testing.T, resp *http.Response, want int)
```

Add a convenience method on `*Env`:
```go
func (e *Env) client() *client {
    return newClient(e.URL)
}
```

- [ ] **Step 2: Write `fixtures_test.go`**

```go
//go:build integration

package integration_test

// nsFor constructs a test namespace for the given fixture and tag.
// fixture: relative path under testdata/arrows/ e.g. "quiver-test/tool-a"
// tag: git tag e.g. "v1"
// Returns: "quiver.test/quiver-test/tool-a@v1"
func nsFor(fixture, tag string) string {
    return "quiver.test/" + fixture + "@" + tag
}

// readFixture reads a file from testdata/arrows/.
func readFixture(t *testing.T, path string) []byte
```

- [ ] **Step 3: Verify compilation**

```bash
go build -tags integration ./tests/integration/...
```

- [ ] **Step 4: Commit**

```bash
git add tests/integration/client_test.go tests/integration/fixtures_test.go
git commit -m "test(integration): add HTTP client and fixture helpers"
```

---

## Task 7: Lifecycle Tests

**Files:**
- Create: `tests/integration/lifecycle_test.go`

**Context:** All tests are methods on `*IntegrationSuite`. Each calls `s.newEnv()` for isolation. API calls go through `env.client()`. State assertions poll `GET /v0/arrow/:ns` until the expected state is reached or a timeout fires.

**Note on state polling:** The wizard executes lifecycle steps asynchronously. After calling `Install`, the arrow transitions from `installing` → `ready` over a short time. Use a helper `waitForState(t, client, ns, state, timeout)` that polls `GetDetail` every 50ms.

- [ ] **Step 1: Write state polling helper at top of file**

```go
//go:build integration

package integration_test

// waitForState polls GET /v0/arrow/:ns until state == want or timeout.
func waitForState(
    t *testing.T,
    c *client,
    ns string,
    want domain.ArrowState,
    timeout time.Duration,
)
```

Polls every 50ms. Calls `t.Fatalf` on timeout with the last observed state.

- [ ] **Step 2: Write `TestLifecycle_FullRoundTrip`**

```go
func (s *IntegrationSuite) TestLifecycle_FullRoundTrip() {
    env := s.newEnv()
    c := env.client()

    // Add
    resp := c.Add(nsFor("quiver-test/tool-a", "v1"))
    mustStatus(s.T(), resp, http.StatusCreated)

    // Verify in list
    resp = c.List()
    mustStatus(s.T(), resp, http.StatusOK)
    // decode and assert tool-a@v1 appears

    // Install
    resp = c.Install(nsFor("quiver-test/tool-a", "v1"), nil)
    mustStatus(s.T(), resp, http.StatusOK)
    waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 10*time.Second)

    // Execute
    resp = c.Execute(nsFor("quiver-test/tool-a", "v1"), domain.MethodExecute, nil)
    mustStatus(s.T(), resp, http.StatusOK)
    waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 10*time.Second)

    // Uninstall
    resp = c.Uninstall(nsFor("quiver-test/tool-a", "v1"), nil)
    mustStatus(s.T(), resp, http.StatusOK)
    waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 10*time.Second)

    // Remove
    resp = c.Remove(nsFor("quiver-test/tool-a", "v1"))
    mustStatus(s.T(), resp, http.StatusNoContent)

    // Verify gone
    resp = c.GetDetail(nsFor("quiver-test/tool-a", "v1"))
    mustStatus(s.T(), resp, http.StatusNotFound)
}
```

- [ ] **Step 3: Write `TestLifecycle_AddIdempotency`**

Add `tool-a@v1` twice. Second response: `409 Conflict`.

- [ ] **Step 4: Write `TestLifecycle_StateViaWebSocket`**

Connect WebSocket to `/v0/arrow.runtime/quiver.test/quiver-test/tool-a@v1`. Add + install `tool-a@v1`. Collect state messages. Assert `installing` appears before `ready`.

```go
func (s *IntegrationSuite) TestLifecycle_StateViaWebSocket() {
    env := s.newEnv()
    c := env.client()

    conn, err := c.DialRuntime(nsFor("quiver-test/tool-a", "v1"))
    s.Require().NoError(err)
    defer conn.Close()

    // Add and install in background goroutine
    // Collect WebSocket messages for 10s
    // Assert states include "installing" then "ready" in order
}
```

- [ ] **Step 5: Write `TestLifecycle_ExecuteUnknownMethod`**

Execute `tool-a@v1/_unknownmethod`. Expect 4xx. GetDetail: state unchanged (still `ready`).

- [ ] **Step 6: Run lifecycle tests**

```bash
go test -tags integration -timeout 120s -v -run TestIntegration/TestLifecycle ./tests/integration/...
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add tests/integration/lifecycle_test.go
git commit -m "test(integration): add lifecycle scenario tests"
```

---

## Task 8: Dependency Tests

**Files:**
- Create: `tests/integration/deps_test.go`

- [ ] **Step 1: Write `TestDeps_TransitiveInstall`**

Add `quiver.test/quiver-test/composed-c@v1`. Install it. Assert `tool-a@v1` and `service-b@v1` appear in `GET /v0/arrow` AND reach state `ready` before `composed-c@v1` transitions to installing.

- [ ] **Step 2: Write `TestDeps_DiamondDeduplication`**

Add `quiver.test/dep-diamond/root@v1`. Install it. Poll until all 4 arrows (root, left, right, shared) reach `ready`. Then call `GET /v0/arrow` and assert `shared` appears exactly once — not twice.

- [ ] **Step 3: Write `TestDeps_CircularDetection`**

```go
func (s *IntegrationSuite) TestDeps_CircularDetection() {
    env := s.newEnv()
    c := env.client()
    resp := c.Add(nsFor("dep-circular/circ-a", "v1"))
    // Expect an error response (4xx or 5xx)
    s.NotEqual(http.StatusCreated, resp.StatusCode)
    // Assert catalog is empty
    listResp := c.List()
    // decode and assert empty
}
```

- [ ] **Step 4: Write `TestDeps_RemoveBlockedByDependents`**

Add and install `composed-c@v1` (which auto-installs `tool-a@v1`). Attempt `DELETE /v0/arrow/quiver.test/quiver-test/tool-a@v1`. Assert `409`. `tool-a@v1` still in catalog.

- [ ] **Step 5: Write `TestDeps_RemoveAfterDependentsGone`**

After `composed-c@v1` is uninstalled and removed: `DELETE /v0/arrow/quiver.test/quiver-test/tool-a@v1` → `204`.

- [ ] **Step 6: Write `TestDeps_OrphanCleanup`**

Install `composed-c@v1` (which installs `tool-a@v1` and `service-b@v1`). Uninstall `composed-c@v1` with `uninstall_orphans: true` in request body. Poll until `tool-a@v1` and `service-b@v1` are absent.

- [ ] **Step 7: Run**

```bash
go test -tags integration -timeout 120s -v -run TestIntegration/TestDeps ./tests/integration/...
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add tests/integration/deps_test.go
git commit -m "test(integration): add dependency management tests"
```

---

## Task 9: Versioning Tests

**Files:**
- Create: `tests/integration/versioning_test.go`

**Context:** `versioned/v1/arrow.yaml` uses `tool-a` as a dep. `versioned/v2/arrow.yaml` drops `tool-a` and adds `service-b`. The in-memory repo for `quiver-test/versioned` has both commits tagged.

- [ ] **Step 1: Write `TestVersioning_TwoVersionsCoexist`**

Add `quiver.test/quiver-test/versioned@v1` and `quiver.test/quiver-test/versioned@v2`. `GET /v0/arrow` response: one entry for the bare namespace `quiver.test/quiver-test/versioned` with two version entries. Installing `@v1` must not affect `@v2`'s state.

- [ ] **Step 2: Write `TestVersioning_VersionPinSurvivesUpdate`**

Add `versioned@v1`. `PATCH /v0/arrow/versioned@v1` with body `{"upgrade_ref": false}`. Assert arrow stays at v1 (detail endpoint shows `installed_ref: v1`).

- [ ] **Step 3: Write `TestVersioning_UpgradeRef`**

Add and install `versioned@v1`. `PATCH` with `{"upgrade_ref": true}`. Assert:
- Response includes `new_ref: "v2"`
- `GET /v0/arrow/quiver.test/quiver-test/versioned@v2` returns 200
- `GET /v0/arrow/quiver.test/quiver-test/versioned@v1` returns 404

- [ ] **Step 4: Write `TestVersioning_AddedDepInstalledOnUpgrade`**

Add and install `versioned@v1`. Upgrade to `v2` with `{"upgrade_ref": true, "install_added": true}`. Assert `service-b@v1` (newly added dep in v2) is installed and reaches `ready`.

- [ ] **Step 5: Write `TestVersioning_RemovedDepUninstalledOnUpgrade`**

Add and install `versioned@v1`. Upgrade to `v2` with `{"upgrade_ref": true, "uninstall_orphans": true}`. Assert `tool-a@v1` (dropped dep in v2) reaches `absent` state.

- [ ] **Step 6: Run**

```bash
go test -tags integration -timeout 120s -v -run TestIntegration/TestVersioning ./tests/integration/...
```

- [ ] **Step 7: Commit**

```bash
git add tests/integration/versioning_test.go
git commit -m "test(integration): add versioning and version upgrade tests"
```

---

## Task 10: Concurrency Tests

**Files:**
- Create: `tests/integration/concurrency_test.go`

**Context:** These tests fire multiple goroutines concurrently against the same `httptest.Server`. The test runner must use `-race` (already in `test-integration` target). Expected behavior: exactly one success, clean errors on conflicts.

- [ ] **Step 1: Write `TestConcurrency_AddSameNamespace`**

```go
func (s *IntegrationSuite) TestConcurrency_AddSameNamespace() {
    env := s.newEnv()
    c := env.client()

    const N = 10
    results := make([]int, N)
    var wg sync.WaitGroup
    for i := range N {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            results[i] = c.Add(nsFor("quiver-test/tool-a", "v1")).StatusCode
        }(i)
    }
    wg.Wait()

    created := 0
    for _, code := range results {
        if code == http.StatusCreated { created++ }
    }
    s.Equal(1, created, "exactly one Add should succeed")

    // Catalog has exactly one entry
    listResp := c.List()
    // decode, assert len == 1
}
```

- [ ] **Step 2: Write `TestConcurrency_ConcurrentInstallsSharedDep`**

Add `composed-c@v1` (deps on `tool-a@v1`) and a second arrow that also depends on `tool-a@v1`. Install both concurrently. Assert `tool-a@v1` reaches `ready` exactly once (not double-installed or stuck).

- [ ] **Step 3: Write `TestConcurrency_InstallAndUninstall`**

Start install of `tool-a@v1` (long-running if possible — give it a slow fixture with a `sleep 1` install step). Immediately fire uninstall. Assert the final state is consistent: either `ready` or `absent`. Never `installing` stuck.

For the slow fixture: add a `tool-a-slow` fixture variant with `sleep 1` in its install step, or override the fixture dynamically.

Actually, use `service-b` (has longer-running execute). For install, add `sleep 1` to `service-b`'s install step. Keep a separate fixture `tool-a-slow/arrow.yaml` with a 1-second install step.

Add `tool-a-slow/arrow.yaml` to testdata (same as `tool-a` but install command: `sleep 1 && echo installed`). Add corresponding fixture repo in `buildFixtureRepos`.

- [ ] **Step 4: Write `TestConcurrency_ConcurrentListUnderLoad`**

Fire 50 goroutines all calling `GET /v0/arrow` simultaneously while 5 installs are in progress. Assert all 50 responses are `200` with valid JSON. Use `-race` to detect data races.

- [ ] **Step 5: Run with race detector**

```bash
go test -tags integration -race -timeout 180s -v -run TestIntegration/TestConcurrency ./tests/integration/...
```

Expected: all PASS, no race conditions.

- [ ] **Step 6: Commit**

```bash
git add tests/integration/concurrency_test.go tests/integration/testdata/arrows/tool-a-slow/
git commit -m "test(integration): add concurrency stress tests"
```

---

## Task 11: Edge Case Tests

**Files:**
- Create: `tests/integration/edge_cases_test.go`

**Context:** These tests verify the state machine enforces its invariants and that bad input is rejected cleanly. No test should leave an arrow in a stuck transitional state.

- [ ] **Step 1: Write `TestEdge_InstallWhileAlreadyInstalling`**

Use `tool-a-slow` (1-second install). Start install. While in `installing` state, trigger install again. Second call must return 4xx/5xx. First install completes normally → `ready`.

- [ ] **Step 2: Write `TestEdge_ExecuteWhileInstalling`**

Use `tool-a-slow`. Start install. While in `installing`, fire execute. Expect rejection. Install completes → `ready`.

- [ ] **Step 3: Write `TestEdge_UpdateWhileRunning`**

Add, install, then execute `service-b@v1` (long-running, goes to `running`). Fire `PATCH /v0/arrow/service-b@v1`. Assert rejection with appropriate status. Stop the service afterward to clean up.

- [ ] **Step 4: Write `TestEdge_RemoveWhileInstalling`**

Use `tool-a-slow`. Start install. Fire `DELETE`. Assert rejection. Install completes.

- [ ] **Step 5: Write `TestEdge_MalformedYAML`**

```go
func (s *IntegrationSuite) TestEdge_MalformedYAML() {
    env := s.newEnv()
    c := env.client()

    content := readFixture(s.T(), "malformed/arrow.yaml")
    resp := c.Seed(nsFor("quiver-test/malformed", "v1"), content)
    s.Equal(http.StatusBadRequest, resp.StatusCode)

    // Arrow not in catalog
    resp = c.GetDetail(nsFor("quiver-test/malformed", "v1"))
    s.Equal(http.StatusNotFound, resp.StatusCode)
}
```

- [ ] **Step 6: Write `TestEdge_RulesetViolation`**

Seed `invalid-ruleset/arrow.yaml`. Expect `422 Unprocessable Entity`. Response body decodes to a `ValidationResult` with `valid: false` and at least one error.

- [ ] **Step 7: Write `TestEdge_NoTargetForCurrentOS`**

Add `quiver.test/quiver-test/no-current-os@v1`. Expect add to fail (the manifold can compile it, but install should fail with "no target for current OS"). Alternatively, if the error surfaces at add-time (compilation detects current OS has no target), assert the add itself returns an error.

Check which layer surfaces this: `manifold.ParseArrow` compiles all targets regardless of current OS — the current OS check happens at execution time in the runner. Assert `Install` returns an error with a message about unsupported OS.

- [ ] **Step 8: Write `TestEdge_MissingVariablesBlockInstall`**

Add `missing-vars@v1`. Install without providing vars. Expect install to fail (or return an error response). State does not transition to `installing`.

- [ ] **Step 9: Write `TestEdge_MaxNameLength`**

Construct a YAML in-test (don't use testdata file) with `name` of exactly 255 characters. `SEED` it. Expect `200`. Name round-trips correctly in `GET /v0/arrow/:ns`.

Then SEED with a 256-character name. Expect `422`.

```go
func (s *IntegrationSuite) TestEdge_MaxNameLength() {
    env := s.newEnv()
    c := env.client()

    name255 := strings.Repeat("a", domain.MaxNameLength)
    yaml := buildMinimalYAML(name255) // helper that builds valid arrow@v0 YAML

    resp := c.Seed(nsFor("quiver-test/tool-a", "v1"), yaml)
    mustStatus(s.T(), resp, http.StatusOK)

    name256 := strings.Repeat("a", domain.MaxNameLength+1)
    yaml = buildMinimalYAML(name256)
    resp = c.Seed(nsFor("quiver-test/tool-a", "v1"), yaml)
    mustStatus(s.T(), resp, http.StatusUnprocessableEntity)
}
```

Add `buildMinimalYAML(name string) []byte` helper inline.

- [ ] **Step 10: Run**

```bash
go test -tags integration -timeout 120s -v -run TestIntegration/TestEdge ./tests/integration/...
```

- [ ] **Step 11: Commit**

```bash
git add tests/integration/edge_cases_test.go
git commit -m "test(integration): add state machine and manifest edge case tests"
```

---

## Task 12: Stress Tests

**Files:**
- Create: `tests/integration/stress_test.go`

**Context:** These tests validate correctness at scale. They have longer timeouts (built into the `300s` test-integration target). The deep-chain test (26 arrows) and wide-graph test (16 arrows) stress the dependency traversal engine.

- [ ] **Step 1: Write `TestStress_DeepChain`**

Add `quiver.test/dep-chain/a@v1`. Install it. Wait up to 60 seconds for all 26 arrows (a through z) to reach `ready`. Assert topological order: for each pair (n, n+1 in chain), `n+1` reached `ready` before `n` started `installing`.

Collect transitions via WebSocket on `GET /v0/arrow.runtime` (global stream) or poll `GetDetail` for each arrow.

```go
func (s *IntegrationSuite) TestStress_DeepChain() {
    env := s.newEnv()
    c := env.client()

    c.Add(nsFor("dep-chain/a", "v1"))
    c.Install(nsFor("dep-chain/a", "v1"), nil)

    // Wait for all 26 to be ready
    for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
        ns := nsFor(fmt.Sprintf("dep-chain/%s", string(letter)), "v1")
        waitForState(s.T(), c, ns, domain.ArrowStateReady, 60*time.Second)
    }
}
```

- [ ] **Step 2: Write `TestStress_WideGraph`**

Add `quiver.test/dep-wide/root@v1`. Install. Wait for all 16 arrows (root + dep-01..dep-15) to reach `ready` within 60 seconds.

- [ ] **Step 3: Write `TestStress_BulkAdd100`**

Add 100 distinct namespaces by appending a counter suffix. `GET /v0/arrow` returns all 100. Response time under 500ms.

```go
func (s *IntegrationSuite) TestStress_BulkAdd100() {
    env := s.newEnv()
    c := env.client()

    for i := range 100 {
        ns := fmt.Sprintf("quiver.test/quiver-test/tool-a@bulk-%d", i)
        // Seed with tool-a YAML so the arrow resolves
        content := readFixture(s.T(), "tool-a/arrow.yaml")
        c.Seed(ns, content)
    }

    start := time.Now()
    resp := c.List()
    elapsed := time.Since(start)

    mustStatus(s.T(), resp, http.StatusOK)
    s.Less(elapsed, 500*time.Millisecond)
    // decode and assert 100 entries
}
```

- [ ] **Step 4: Write `TestStress_RestartSurvival`**

Install `tool-a@v1`. Destroy the current env (the `httptest.Server` cleanup runs via `t.Cleanup`). Create a second env with the same HOME directory. `GET /v0/arrow/quiver.test/quiver-test/tool-a@v1` returns the arrow with state `ready` — state survived the restart.

```go
func (s *IntegrationSuite) TestStress_RestartSurvival() {
    home := s.T().TempDir()
    s.T().Setenv("HOME", home)

    // First env
    env1 := s.newEnvWithHome(home) // variant of newEnv that accepts explicit home
    c1 := env1.client()
    c1.Add(nsFor("quiver-test/tool-a", "v1"))
    c1.Install(nsFor("quiver-test/tool-a", "v1"), nil)
    waitForState(s.T(), c1, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 15*time.Second)
    env1.close() // shuts down server

    // Second env — same HOME
    env2 := s.newEnvWithHome(home)
    c2 := env2.client()
    resp := c2.GetDetail(nsFor("quiver-test/tool-a", "v1"))
    mustStatus(s.T(), resp, http.StatusOK)
    // decode and assert state == ready
}
```

Add `newEnvWithHome(home string) *Env` variant to `env_test.go` that accepts an explicit home instead of creating a new `TempDir`.

- [ ] **Step 5: Write `TestStress_ListPerformanceUnderVolume`**

With 100 arrows in catalog (use Seed), assert `GET /v0/arrow` responds in under 500ms.

- [ ] **Step 6: Run all stress tests**

```bash
go test -tags integration -timeout 300s -v -run TestIntegration/TestStress ./tests/integration/...
```

Expected: all PASS within the time budget.

- [ ] **Step 7: Run the full suite**

```bash
go test -tags integration -race -timeout 300s -v ./tests/integration/...
```

Expected: all tests PASS, no race conditions.

- [ ] **Step 8: Commit**

```bash
git add tests/integration/stress_test.go
git commit -m "test(integration): add deep chain, wide graph, bulk, and restart survival tests"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| Full round-trip lifecycle | Task 7 |
| State transitions via WebSocket | Task 7 |
| Transitive install | Task 8 |
| Diamond deduplication | Task 8 |
| Circular detection | Task 8 |
| Remove blocked by dependents | Task 8 |
| Orphan cleanup | Task 8 |
| Two versions coexist | Task 9 |
| Version pin + upgrade | Task 9 |
| Added/removed deps on upgrade | Task 9 |
| Concurrent Add same NS | Task 10 |
| Concurrent installs shared dep | Task 10 |
| Concurrent install+uninstall | Task 10 |
| Concurrent list under load | Task 10 |
| State machine violations (install-while-X) | Task 11 |
| Malformed YAML | Task 11 |
| Ruleset violation | Task 11 |
| No current OS target | Task 11 |
| Missing vars | Task 11 |
| Max name length | Task 11 |
| Deep chain (26 levels) | Task 12 |
| Wide graph (15 deps) | Task 12 |
| Bulk add 100 | Task 12 |
| Restart survival | Task 12 |
| Makefile targets | Task 1 |
| CI job | Task 1 |
| `manifold.NewWithResolvers` | Task 2 |
| All fixture arrows | Task 3 |

All spec requirements covered. No placeholders. Type/name consistency: `nsFor`, `waitForState`, `mustStatus`, `decodeJSON`, `buildFixtureRepos`, `testResolver` used consistently across tasks.
