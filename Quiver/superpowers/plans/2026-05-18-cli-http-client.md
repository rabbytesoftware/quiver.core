# CLI HTTP Client Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the concrete `HTTPClient` that implements `QuiverClient` over HTTP REST and WebSocket, set up the `quiver-core/` working directory from a fresh clone, and establish the stacked-branch PR strategy for independent review of the interface and implementation.

**Architecture:** Two PRs — `feature/cli-client` (interface, already committed) is the base; `feature/cli-http-client` (implementation) stacks on top of it and targets it. The implementation adds `ws.go` (WebSocket dial + pump with terminal-state detection) and `http.go` (`HTTPClient` struct + all 23 interface methods). The `cli/` directory is a standalone Go module (`quiver-cli`) separate from the daemon's `go.mod`.

**Tech Stack:** Go 1.26.2, `github.com/gorilla/websocket v1.5.3`, `github.com/stretchr/testify v1.11.1`

---

## Context for the developer

| File | Why |
|---|---|
| `docs/superpowers/specs/2026-05-11-cli-client-interface-design.md` | Approved interface design — method signatures, channel contract, types |
| `internal/api/v0/dto/` | Server DTO shapes and JSON tags the CLI types must mirror |
| `internal/api/v0/endpoints/arrows/handlers/handlers_test.go` | Contains `encodedNS = "/v0/arrow/github.com%2Fuser%2Frepo"` — namespaces are `url.PathEscape`-encoded in all paths |
| `internal/api/libs/response.go` | API envelope: `{"success":bool,"error":*string,"data":any}` — **health is the exception**: `GET /v0/health` returns `{"status":"ok"}` directly, not wrapped |

**Endpoint reference** (all under `/v0`):

| Operation | Method | Path | Body |
|---|---|---|---|
| Arrow list | GET | `/arrow?user_installed=<bool>` | — |
| Arrow detail | GET | `/arrow/:ns` | — |
| Arrow manifest | GET | `/arrow/:ns/manifest` | — |
| Arrow add | POST | `/arrow/:ns` | — |
| Arrow update | PATCH | `/arrow/:ns` | — |
| Arrow remove | DELETE | `/arrow/:ns` | — |
| Arrow seed | POST | `/arrow/:ns/manifest` | raw YAML (`application/x-yaml`) |
| Arrow validate | POST | `/arrow/:ns/manifest/validate` | raw YAML; 200=valid, 422=invalid, both same body |
| Runtime lifecycle | POST | `/runtime/:ns/<method>` | `{"variables":{...}}` |
| Runtime WS (single) | GET (WS) | `/runtime/:ns` | — |
| Runtime WS (all) | GET (WS) | `/runtime` | — |
| Collection list | GET | `/quiver` | — |
| Collection get | GET | `/quiver/:ns` | — |
| Collection add | POST | `/quiver/:ns` | — |
| Collection update | PATCH | `/quiver/:ns` | — |
| Collection remove | DELETE | `/quiver/:ns` | — |
| Health | GET | `/health` | — |

**Lifecycle method names** (the `<method>` segment in the URL):

| `QuiverClient` method | URL method | WS terminal state |
|---|---|---|
| `Install` | `install` | `state == "ready"` or `"absent"` |
| `Uninstall` | `uninstall` | `state == "removed"` or `"ready"` |
| `Run` | `execute` | `state == "ready"` |
| `Stop` | `stop` | `state == "ready"` |
| `Update` | `update` | `state == "ready"` |
| `RunMethod(_, method, _)` | `<method>` | `ActiveRun == nil` after having been non-nil |
| `WatchRuntime` | — (WS only) | ctx cancelled |

**WS stream**: The server broadcasts full `ArrowRuntime` state snapshots on every state change — not discrete step events. Closing the WS connection unblocks `ReadMessage`, which is how context cancellation is handled.

**`RuntimeGet` / `RuntimeList`**: No REST endpoint exists. Both dial WS, read one snapshot, close. `RuntimeList` reads from `/v0/runtime` (all runtimes) and returns the first snapshot as a one-element slice. This blocks until the next state change — acceptable given the design.

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `cli/client/types.go` | **Modify** | Add JSON struct tags so snake_case server JSON decodes into CLI types |
| `cli/client/ws.go` | **Create** | `pump`: dial WS, decode `ArrowRuntime` snapshots, close on terminal state or ctx cancel; stop-condition functions |
| `cli/client/ws_test.go` | **Create** | Unit tests for `pump` using a loopback WS test server (package `client`) |
| `cli/client/http.go` | **Create** | `HTTPClient` struct + `NewHTTPClient` + all 23 `QuiverClient` methods + helpers |
| `cli/client/http_test.go` | **Create** | Unit tests for `HTTPClient` using `httptest.NewServer` (package `client_test`) |

---

## Task 1: Repository Cleanup and Fresh Clone

**Files:** none (filesystem + git ops only)

- [ ] **Step 1: Push `feature/cli-client` from `quiver-cli-interface/`**

```bash
cd /home/valen/Documents/projects/quiver/quiver-cli-interface && git push -u origin feature/cli-client
```

Expected: branch pushed, tracking set.

- [ ] **Step 2: Verify the push landed on remote**

```bash
cd /home/valen/Documents/projects/quiver/quiver-cli-interface && git log --oneline origin/feature/cli-client | head -3
```

Expected: `8475175 feat(cli/client): add QuiverClient interface, types, and FakeClient test double` at top.

- [ ] **Step 3: Clone fresh from origin first (before deleting old directories)**

```bash
git clone https://github.com/rabbytesoftware/quiver.git /home/valen/Documents/projects/quiver/quiver-core
```

Expected: `Cloning into '/home/valen/Documents/projects/quiver/quiver-core'...` — success.

- [ ] **Step 4: Copy the plan file into the new clone before deleting the old directories**

`docs/superpowers/` is gitignored and local-only. Preserve the plan:

```bash
mkdir -p /home/valen/Documents/projects/quiver/quiver-core/docs/superpowers/plans && \
cp /home/valen/Documents/projects/quiver/quiver-cli-interface/docs/superpowers/plans/2026-05-18-cli-http-client.md \
   /home/valen/Documents/projects/quiver/quiver-core/docs/superpowers/plans/
```

Expected: no output.

- [ ] **Step 5: Delete the stale local clones**

```bash
rm -rf /home/valen/Documents/projects/quiver/quiver /home/valen/Documents/projects/quiver/quiver-cli-interface
```

Expected: no output.

- [ ] **Step 6: Check out the interface branch**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && git checkout feature/cli-client
```

Expected: `Switched to a new branch 'feature/cli-client'`

- [ ] **Step 7: Verify cli/client is present**

```bash
ls /home/valen/Documents/projects/quiver/quiver-core/cli/client/
```

Expected: `client.go  fake.go  fake_test.go  types.go`

---

## Task 2: Open Interface PR

**Files:** none

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Create the PR**

```bash
gh pr create \
  --base develop \
  --head feature/cli-client \
  --title "feat(cli/client): QuiverClient interface, types, and FakeClient" \
  --body "$(cat <<'EOF'
## Summary
- Defines `QuiverClient` — the interface boundary between `cmd/` and the HTTP+WS transport layer
- Defines all shared CLI types that mirror the server DTO JSON shapes
- Implements `FakeClient` (function-field test double) and `StreamOf` helper for unit-testing `cmd/` without a running daemon

## Test plan
- [ ] `cd cli && go test ./client/ -v -race` — all 6 unit tests pass
- [ ] Review interface shape against `docs/superpowers/specs/2026-05-11-cli-client-interface-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed.

- [ ] **Step 2: Note the PR URL for the implementation PR description**

---

## Task 3: Create the Implementation Branch

**Files:** none

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Cut the implementation branch from the interface branch**

```bash
git checkout -b feature/cli-http-client
```

Expected: `Switched to a new branch 'feature/cli-http-client'`

- [ ] **Step 2: Verify lineage**

```bash
git log --oneline feature/cli-http-client ^develop | head -5
```

Expected: at least `8475175 feat(cli/client): add QuiverClient interface, types, and FakeClient test double` — the interface commit is in this branch's history.

---

## Task 4: Add JSON Tags + gorilla/websocket Dependency

**Files:**
- Modify: `cli/client/types.go`

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Rewrite `cli/client/types.go` with JSON struct tags**

The server uses snake_case JSON. Without tags, `active_run` in JSON would not decode into `ActiveRun`. Replace the entire file:

```go
package client

type ArrowRuntime struct {
	Namespace  string     `json:"namespace"`
	State      string     `json:"state"`
	ActiveRun  *RunRecord `json:"active_run"`
	LastReturn *Return    `json:"last_return"`
}

type RunRecord struct {
	Method    string            `json:"method"`
	PID       int               `json:"pid"`
	Variables map[string]string `json:"variables"`
	Steps     []StepProgress    `json:"steps"`
}

type StepProgress struct {
	Index  int     `json:"index"`
	Status string  `json:"status"`
	Title  string  `json:"title"`
	Type   string  `json:"type"`
	Error  *string `json:"error"`
}

type Return struct {
	Method    string            `json:"method"`
	Outcome   string            `json:"outcome"`
	Variables map[string]string `json:"variables"`
	Steps     []StepProgress    `json:"steps"`
}

type ArrowListItem struct {
	Namespace   string             `json:"namespace"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Tags        []string           `json:"tags"`
	Versions    []InstalledVersion `json:"versions"`
}

type InstalledVersion struct {
	Ref         string `json:"ref"`
	Version     string `json:"version"`
	State       string `json:"state"`
	InstalledAt string `json:"installed_at"`
	Constraint  string `json:"constraint"`
}

type ArrowDetail struct {
	Namespace           string     `json:"namespace"`
	Name                string     `json:"name"`
	Version             string     `json:"version"`
	Description         string     `json:"description"`
	License             string     `json:"license"`
	State               string     `json:"state"`
	Tags                []string   `json:"tags"`
	InstalledRef        string     `json:"installed_ref"`
	InstalledAt         string     `json:"installed_at"`
	InstalledConstraint string     `json:"installed_constraint"`
	UserInstalled       bool       `json:"user_installed"`
	ActiveRun           *RunRecord `json:"active_run"`
	LastReturn          *Return    `json:"last_return"`
}

type ValidationResult struct {
	Valid                bool              `json:"valid"`
	Errors               []ValidationError `json:"errors"`
	SupportedPlatforms   []string          `json:"supported_platforms"`
	UnsupportedPlatforms []string          `json:"unsupported_platforms"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type Collection struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type HealthStatus struct {
	Status string `json:"status"`
}
```

- [ ] **Step 2: Run existing tests — verify no regressions**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race
```

Expected: all 6 existing tests pass.

- [ ] **Step 3: Add gorilla/websocket**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go get github.com/gorilla/websocket@v1.5.3 && go mod tidy
```

Expected: `go: added github.com/gorilla/websocket v1.5.3`

- [ ] **Step 4: Verify go.mod**

```bash
grep websocket /home/valen/Documents/projects/quiver/quiver-core/cli/go.mod
```

Expected: `github.com/gorilla/websocket v1.5.3`

- [ ] **Step 5: Commit**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && \
git add cli/client/types.go cli/go.mod cli/go.sum && \
git commit -m "$(cat <<'EOF'
feat(cli/client): add JSON struct tags to types; add gorilla/websocket dep

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: WebSocket Pump

**Files:**
- Create: `cli/client/ws_test.go`
- Create: `cli/client/ws.go`

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Write the failing tests**

Create `cli/client/ws_test.go` (package `client` — tests unexported `pump`):

```go
package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// wsTestServer starts a WS server that sends msgs then waits for the client to disconnect.
func wsTestServer(t *testing.T, msgs []ArrowRuntime) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for _, m := range msgs {
			data, err := json.Marshal(m)
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestPump_TerminatesOnTerminalState(t *testing.T) {
	msgs := []ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
		{Namespace: "github.com/foo/bar", State: "ready"},
		{Namespace: "github.com/foo/bar", State: "ready"}, // should not be received — channel closed after second
	}
	url := wsTestServer(t, msgs)

	ch, err := pump(context.Background(), url, terminalInstall)
	require.NoError(t, err)

	var got []ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}

	require.Len(t, got, 2)
	assert.Equal(t, "installing", got[0].State)
	assert.Equal(t, "ready", got[1].State)
}

func TestPump_TerminatesOnCtxCancel(t *testing.T) {
	url := wsTestServer(t, []ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
	})

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := pump(ctx, url, neverStop)
	require.NoError(t, err)

	rt, ok := <-ch
	require.True(t, ok)
	assert.Equal(t, "installing", rt.State)

	cancel()

	_, ok = <-ch
	assert.False(t, ok, "channel must close after ctx cancellation")
}

func TestPump_TerminalCustomMethod(t *testing.T) {
	activeRun := &RunRecord{Method: "deploy"}
	msgs := []ArrowRuntime{
		{Namespace: "ns", State: "running", ActiveRun: activeRun},
		{Namespace: "ns", State: "ready"},
	}
	url := wsTestServer(t, msgs)

	ch, err := pump(context.Background(), url, terminalCustomMethod)
	require.NoError(t, err)

	var got []ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}

	require.Len(t, got, 2)
	assert.NotNil(t, got[0].ActiveRun)
	assert.Nil(t, got[1].ActiveRun)
}

func TestPump_ConnectionError_ReturnsErr(t *testing.T) {
	_, err := pump(context.Background(), "ws://127.0.0.1:1", neverStop)
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run — verify it fails to compile**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ 2>&1 | head -5
```

Expected: compile error — `undefined: pump`

- [ ] **Step 3: Create `cli/client/ws.go`**

```go
package client

import (
	"context"
	"encoding/json"

	"github.com/gorilla/websocket"
)

// pump dials wsURL and delivers ArrowRuntime snapshots to the returned channel.
// The channel is closed when stopFn returns true or ctx is cancelled.
// stopFn receives the snapshot and whether ActiveRun has ever been non-nil in this stream.
// Closing the WS connection (on ctx cancel) unblocks ReadMessage — the gorilla pattern
// for context-aware WS reads.
func pump(ctx context.Context, wsURL string, stopFn func(rt ArrowRuntime, sawRun bool) bool) (<-chan ArrowRuntime, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}

	ch := make(chan ArrowRuntime, 16)

	go func() {
		defer conn.Close()
		defer close(ch)

		go func() {
			<-ctx.Done()
			conn.Close()
		}()

		sawRun := false
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var rt ArrowRuntime
			if err := json.Unmarshal(msg, &rt); err != nil {
				continue
			}

			if rt.ActiveRun != nil {
				sawRun = true
			}

			select {
			case ch <- rt:
			case <-ctx.Done():
				return
			}

			if stopFn(rt, sawRun) {
				return
			}
		}
	}()

	return ch, nil
}

// Terminal-state functions — one per lifecycle method category.

func terminalInstall(rt ArrowRuntime, _ bool) bool {
	return rt.State == "ready" || rt.State == "absent"
}

func terminalUninstall(rt ArrowRuntime, _ bool) bool {
	return rt.State == "removed" || rt.State == "ready"
}

func terminalReady(rt ArrowRuntime, _ bool) bool {
	return rt.State == "ready"
}

// terminalCustomMethod closes after ActiveRun goes nil following a non-nil snapshot.
func terminalCustomMethod(rt ArrowRuntime, sawRun bool) bool {
	return sawRun && rt.ActiveRun == nil
}

func neverStop(_ ArrowRuntime, _ bool) bool { return false }
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race -run TestPump
```

Expected:
```
--- PASS: TestPump_TerminatesOnTerminalState
--- PASS: TestPump_TerminatesOnCtxCancel
--- PASS: TestPump_TerminalCustomMethod
--- PASS: TestPump_ConnectionError_ReturnsErr
PASS
```

- [ ] **Step 5: Run all client tests to verify no regressions**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race
```

Expected: all 10 tests pass (6 existing + 4 new).

- [ ] **Step 6: Commit**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && \
git add cli/client/ws.go cli/client/ws_test.go && \
git commit -m "$(cat <<'EOF'
feat(cli/client): add WebSocket pump with terminal-state detection

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: HTTPClient — Arrow Catalog Methods

**Files:**
- Create: `cli/client/http_test.go` (Arrow tests)
- Create: `cli/client/http.go` (struct + helpers + Arrow methods)

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Write the failing Arrow tests**

Create `cli/client/http_test.go`:

```go
package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"quiver-cli/client"
)

// apiOK writes a standard success envelope around data.
func apiOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

// apiErr writes a standard error envelope.
func apiErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
}

// httpClient creates a test HTTP server with the given handler and returns an HTTPClient pointing at it.
func httpClient(t *testing.T, handler http.HandlerFunc) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.NewHTTPClient(srv.URL)
}

// --- ArrowList ---

func TestHTTPClient_ArrowList_ReturnsItems(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v0/arrow", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("user_installed"))
		apiOK(w, []client.ArrowListItem{
			{Namespace: "github.com/foo/bar", Name: "bar"},
		})
	})

	items, err := c.ArrowList(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "github.com/foo/bar", items[0].Namespace)
}

func TestHTTPClient_ArrowList_ServerError_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusInternalServerError, "internal error")
	})
	_, err := c.ArrowList(context.Background(), false)
	assert.Error(t, err)
}

// --- ArrowGet ---

func TestHTTPClient_ArrowGet_ReturnsDetail(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.Path)
		apiOK(w, client.ArrowDetail{Namespace: "github.com/foo/bar", State: "ready"})
	})

	detail, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "ready", detail.State)
}

func TestHTTPClient_ArrowGet_NotFound_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusNotFound, "not found")
	})
	_, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	assert.Error(t, err)
}

// --- ArrowGetManifest ---

func TestHTTPClient_ArrowGetManifest_ReturnsDataBytes(t *testing.T) {
	manifest := map[string]any{"name": "bar", "version": "1.0.0"}
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.Path)
		apiOK(w, manifest)
	})

	data, err := c.ArrowGetManifest(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "bar", decoded["name"])
}

// --- ArrowAdd ---

func TestHTTPClient_ArrowAdd_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true, "namespace": "github.com/foo/bar"})
	})
	assert.NoError(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

func TestHTTPClient_ArrowAdd_Conflict_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusConflict, "already registered")
	})
	assert.Error(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

// --- ArrowUpdate ---

func TestHTTPClient_ArrowUpdate_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowUpdate(context.Background(), "github.com/foo/bar"))
}

// --- ArrowRemove ---

func TestHTTPClient_ArrowRemove_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar%40v1.0.0", r.URL.Path)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowRemove(context.Background(), "github.com/foo/bar@v1.0.0"))
}

// --- ArrowSeed ---

func TestHTTPClient_ArrowSeed_SendsYAMLBody(t *testing.T) {
	manifest := []byte("name: bar\nversion: 1.0.0")
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.Path)
		assert.Equal(t, "application/x-yaml", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowSeed(context.Background(), "github.com/foo/bar", manifest))
}

// --- ArrowValidate ---

func TestHTTPClient_ArrowValidate_Valid(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest/validate", r.URL.Path)
		apiOK(w, client.ValidationResult{Valid: true, SupportedPlatforms: []string{"linux"}})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("name: bar"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
}

func TestHTTPClient_ArrowValidate_Invalid_ReturnsResult(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Server returns 422 for invalid but same body shape.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data": client.ValidationResult{
				Valid:  false,
				Errors: []client.ValidationError{{Field: "name", Rule: "required", Message: "name is required"}},
			},
		})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("version: 1.0.0"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "name", result.Errors[0].Field)
}
```

- [ ] **Step 2: Run — verify it fails to compile**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ 2>&1 | head -5
```

Expected: compile error — `undefined: client.HTTPClient` or `client.NewHTTPClient`

- [ ] **Step 3: Create `cli/client/http.go`** with the full struct, helpers, and Arrow methods

```go
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient implements QuiverClient over HTTP REST and WebSocket.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewHTTPClient returns an HTTPClient targeting baseURL (e.g. "http://localhost:8080").
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// apiEnvelope is the server's standard response wrapper.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Error   *string         `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// getJSON sends GET to path and decodes the data field as T.
func getJSON[T any](ctx context.Context, c *HTTPClient, path string) (T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		var zero T
		return zero, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var zero T
		return zero, err
	}
	defer resp.Body.Close()
	return decodeEnvelope[T](resp.Body)
}

// decodeEnvelope decodes an apiEnvelope and unmarshals data as T.
func decodeEnvelope[T any](r io.Reader) (T, error) {
	var env apiEnvelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		var zero T
		return zero, err
	}
	if !env.Success {
		var zero T
		msg := "request failed"
		if env.Error != nil {
			msg = *env.Error
		}
		return zero, fmt.Errorf("%s", msg)
	}
	var result T
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// mutate sends a mutation request (no body or JSON body) and returns an error on HTTP >= 400.
func (c *HTTPClient) mutate(ctx context.Context, method, path string, body any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var env apiEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if env.Error != nil {
			msg = *env.Error
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// wsURL converts the HTTP base URL to its WebSocket equivalent.
func (c *HTTPClient) wsURL(path string) string {
	switch {
	case strings.HasPrefix(c.baseURL, "https://"):
		return "wss://" + strings.TrimPrefix(c.baseURL, "https://") + path
	default:
		return "ws://" + strings.TrimPrefix(c.baseURL, "http://") + path
	}
}

// ns returns the URL-path-escaped form of a namespace.
// e.g. "github.com/foo/bar" → "github.com%2Ffoo%2Fbar"
func ns(namespace string) string {
	return url.PathEscape(namespace)
}

// --- Arrow catalog ---

func (c *HTTPClient) ArrowList(ctx context.Context, userInstalled bool) ([]ArrowListItem, error) {
	return getJSON[[]ArrowListItem](ctx, c, fmt.Sprintf("/v0/arrow?user_installed=%v", userInstalled))
}

func (c *HTTPClient) ArrowGet(ctx context.Context, namespace string) (*ArrowDetail, error) {
	result, err := getJSON[ArrowDetail](ctx, c, "/v0/arrow/"+ns(namespace))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPClient) ArrowGetManifest(ctx context.Context, namespace string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.Success {
		msg := "request failed"
		if env.Error != nil {
			msg = *env.Error
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return env.Data, nil
}

func (c *HTTPClient) ArrowAdd(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPost, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowUpdate(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPatch, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowRemove(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodDelete, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowSeed(ctx context.Context, namespace string, manifest []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest", bytes.NewReader(manifest))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var env apiEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if env.Error != nil {
			msg = *env.Error
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (c *HTTPClient) ArrowValidate(ctx context.Context, namespace string, manifest []byte) (*ValidationResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest/validate", bytes.NewReader(manifest))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Server returns 200 for valid, 422 for invalid — both use the same envelope shape.
	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	var result ValidationResult
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
```

- [ ] **Step 4: Run Arrow tests — verify they pass**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race -run TestHTTPClient_Arrow
```

Expected: all 11 Arrow tests pass.

- [ ] **Step 5: Commit**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && \
git add cli/client/http.go cli/client/http_test.go && \
git commit -m "$(cat <<'EOF'
feat(cli/client): HTTPClient struct, helpers, and Arrow catalog methods

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: HTTPClient — Runtime Methods

**Files:**
- Modify: `cli/client/http_test.go` (add Runtime tests)
- Modify: `cli/client/http.go` (add Runtime methods)

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Add the failing Runtime tests to `http_test.go`**

Append to `cli/client/http_test.go` (after the existing Arrow tests):

```go
// --- Runtime tests ---
// These tests use a combined HTTP+WS server: the POST handler returns 202,
// then the WS handler streams pre-canned ArrowRuntime snapshots.

import (
    // add to existing imports:
    "net/http/httptest" // already imported
    "strings"

    "github.com/gorilla/websocket"
)

var testWSUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// lifecycleServer creates a server that accepts POST (returns 202) and WS (streams snapshots).
func lifecycleServer(t *testing.T, snapshots []client.ArrowRuntime) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, err := testWSUpgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer conn.Close()
			for _, s := range snapshots {
				data, err := json.Marshal(s)
				require.NoError(t, err)
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		} else {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		}
	}))
	t.Cleanup(srv.Close)
	return client.NewHTTPClient(srv.URL)
}

func TestHTTPClient_Install_StreamsSnapshots(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
		{Namespace: "github.com/foo/bar", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Install(context.Background(), "github.com/foo/bar", nil)
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.Equal(t, "installing", got[0].State)
	assert.Equal(t, "ready", got[1].State)
}

func TestHTTPClient_Uninstall_ClosesOnRemoved(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "uninstalling"},
		{Namespace: "github.com/foo/bar", State: "removed"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Uninstall(context.Background(), "github.com/foo/bar", nil)
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.Equal(t, "removed", got[1].State)
}

func TestHTTPClient_Run_ClosesOnReady(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "ns", State: "running"},
		{Namespace: "ns", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Run(context.Background(), "ns", map[string]string{"key": "val"})
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	assert.Equal(t, "ready", got[len(got)-1].State)
}

func TestHTTPClient_Stop_ClosesOnReady(t *testing.T) {
	c := lifecycleServer(t, []client.ArrowRuntime{{Namespace: "ns", State: "ready"}})
	ch, err := c.Stop(context.Background(), "ns")
	require.NoError(t, err)
	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 1)
	assert.Equal(t, "ready", got[0].State)
}

func TestHTTPClient_RunMethod_ClosesAfterActiveRunClears(t *testing.T) {
	activeRun := &client.RunRecord{Method: "deploy"}
	snapshots := []client.ArrowRuntime{
		{Namespace: "ns", State: "running", ActiveRun: activeRun},
		{Namespace: "ns", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.RunMethod(context.Background(), "ns", "deploy", nil)
	require.NoError(t, err)
	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.NotNil(t, got[0].ActiveRun)
	assert.Nil(t, got[1].ActiveRun)
}

func TestHTTPClient_WatchRuntime_ClosesOnCtxCancel(t *testing.T) {
	// Send one snapshot; the client cancels after receiving it.
	snapshots := []client.ArrowRuntime{{Namespace: "ns", State: "ready"}}
	c := lifecycleServer(t, snapshots)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.WatchRuntime(ctx, "ns")
	require.NoError(t, err)

	rt, ok := <-ch
	require.True(t, ok)
	assert.Equal(t, "ready", rt.State)

	cancel()
	_, ok = <-ch
	assert.False(t, ok)
}

func TestHTTPClient_RuntimeGet_ReturnsSingleSnapshot(t *testing.T) {
	snapshots := []client.ArrowRuntime{{Namespace: "ns", State: "ready"}}
	c := lifecycleServer(t, snapshots) // only WS path is exercised here

	rt, err := c.RuntimeGet(context.Background(), "ns")
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, "ready", rt.State)
}
```

- [ ] **Step 2: Run — verify the new tests fail to compile**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ 2>&1 | head -10
```

Expected: compile error — methods `Install`, `Uninstall`, etc. undefined on `HTTPClient`.

- [ ] **Step 3: Add Runtime methods to `cli/client/http.go`**

Append to the end of `cli/client/http.go`:

```go
// --- Runtime lifecycle ---

// postRuntime fires POST /v0/runtime/:ns/<method> with optional variables.
func (c *HTTPClient) postRuntime(ctx context.Context, namespace, method string, vars map[string]string) error {
	type body struct {
		Variables map[string]string `json:"variables,omitempty"`
	}
	path := "/v0/runtime/" + ns(namespace) + "/" + url.PathEscape(method)
	return c.mutate(ctx, http.MethodPost, path, body{Variables: vars})
}

// lifecycle fires POST then opens the WS stream for namespace.
func (c *HTTPClient) lifecycle(ctx context.Context, namespace, method string, vars map[string]string, stopFn func(ArrowRuntime, bool) bool) (<-chan ArrowRuntime, error) {
	if err := c.postRuntime(ctx, namespace, method, vars); err != nil {
		return nil, err
	}
	return pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), stopFn)
}

func (c *HTTPClient) Install(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "install", vars, terminalInstall)
}

func (c *HTTPClient) Uninstall(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "uninstall", vars, terminalUninstall)
}

func (c *HTTPClient) Run(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "execute", vars, terminalReady)
}

func (c *HTTPClient) Stop(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "stop", nil, terminalReady)
}

func (c *HTTPClient) Update(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "update", nil, terminalReady)
}

func (c *HTTPClient) RunMethod(ctx context.Context, namespace, method string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, method, vars, terminalCustomMethod)
}

// --- Runtime observation ---

func (c *HTTPClient) RuntimeGet(ctx context.Context, namespace string) (*ArrowRuntime, error) {
	stopAfterOne := func(_ ArrowRuntime, _ bool) bool { return true }
	ch, err := pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), stopAfterOne)
	if err != nil {
		return nil, err
	}
	rt, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("no runtime snapshot received for %s", namespace)
	}
	return &rt, nil
}

func (c *HTTPClient) RuntimeList(ctx context.Context) ([]ArrowRuntime, error) {
	// /v0/runtime broadcasts all runtime updates. Read the first snapshot.
	stopAfterOne := func(_ ArrowRuntime, _ bool) bool { return true }
	ch, err := pump(ctx, c.wsURL("/v0/runtime"), stopAfterOne)
	if err != nil {
		return nil, err
	}
	rt, ok := <-ch
	if !ok {
		return nil, nil
	}
	return []ArrowRuntime{rt}, nil
}

func (c *HTTPClient) WatchRuntime(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), neverStop)
}
```

- [ ] **Step 4: Fix the import block in http_test.go**

The `lifecycleServer` helper and its tests reference `strings` and `github.com/gorilla/websocket` — these must be in the import block of `http_test.go`. Replace the import block at the top of `cli/client/http_test.go` with:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"quiver-cli/client"
)
```

- [ ] **Step 5: Run Runtime tests — verify they pass**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race -run TestHTTPClient_Install|TestHTTPClient_Uninstall|TestHTTPClient_Run|TestHTTPClient_Stop|TestHTTPClient_RunMethod|TestHTTPClient_WatchRuntime|TestHTTPClient_RuntimeGet
```

Expected: all 7 Runtime tests pass.

- [ ] **Step 6: Commit**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && \
git add cli/client/http.go cli/client/http_test.go && \
git commit -m "$(cat <<'EOF'
feat(cli/client): add runtime lifecycle and observation methods to HTTPClient

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: HTTPClient — Collection, Health, and Interface Assertion

**Files:**
- Modify: `cli/client/http_test.go` (add Collection + Health tests)
- Modify: `cli/client/http.go` (add Collection + Health methods + compile-time assertion)

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Add failing Collection and Health tests to `http_test.go`**

Append to `cli/client/http_test.go`:

```go
// --- Collection ---

func TestHTTPClient_CollectionList_ReturnsItems(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v0/quiver", r.URL.Path)
		apiOK(w, []client.Collection{{Namespace: "github.com/org/set", Name: "set"}})
	})

	items, err := c.CollectionList(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "github.com/org/set", items[0].Namespace)
}

func TestHTTPClient_CollectionGet_ReturnsCollection(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/quiver/github.com%2Forg%2Fset", r.URL.Path)
		apiOK(w, client.Collection{Namespace: "github.com/org/set", Name: "set"})
	})

	col, err := c.CollectionGet(context.Background(), "github.com/org/set")
	require.NoError(t, err)
	require.NotNil(t, col)
	assert.Equal(t, "github.com/org/set", col.Namespace)
}

func TestHTTPClient_CollectionAdd_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/quiver/github.com%2Forg%2Fset", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.CollectionAdd(context.Background(), "github.com/org/set"))
}

func TestHTTPClient_CollectionUpdate_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.CollectionUpdate(context.Background(), "github.com/org/set"))
}

func TestHTTPClient_CollectionRemove_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.CollectionRemove(context.Background(), "github.com/org/set"))
}

// --- Health ---

func TestHTTPClient_Health_ReturnsStatus(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/health", r.URL.Path)
		// Health does NOT use the apiEnvelope wrapper.
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})

	hs, err := c.Health(context.Background())
	require.NoError(t, err)
	require.NotNil(t, hs)
	assert.Equal(t, "ok", hs.Status)
}

func TestHTTPClient_Health_ServerDown_ReturnsErr(t *testing.T) {
	c := client.NewHTTPClient("http://127.0.0.1:1")
	_, err := c.Health(context.Background())
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run — verify the new tests fail to compile**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ 2>&1 | head -5
```

Expected: compile error — `CollectionList` etc. undefined on `HTTPClient`.

- [ ] **Step 3: Add Collection + Health methods to `cli/client/http.go`**

Append to the end of `cli/client/http.go`:

```go
// --- Collections ---
// The server uses /quiver for what the CLI calls Collection.

func (c *HTTPClient) CollectionList(ctx context.Context) ([]Collection, error) {
	return getJSON[[]Collection](ctx, c, "/v0/quiver")
}

func (c *HTTPClient) CollectionGet(ctx context.Context, namespace string) (*Collection, error) {
	result, err := getJSON[Collection](ctx, c, "/v0/quiver/"+ns(namespace))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPClient) CollectionAdd(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPost, "/v0/quiver/"+ns(namespace), nil)
}

func (c *HTTPClient) CollectionUpdate(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPatch, "/v0/quiver/"+ns(namespace), nil)
}

func (c *HTTPClient) CollectionRemove(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodDelete, "/v0/quiver/"+ns(namespace), nil)
}

// --- System ---

func (c *HTTPClient) Health(ctx context.Context) (*HealthStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v0/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var hs HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&hs); err != nil {
		return nil, err
	}
	return &hs, nil
}

// Compile-time proof that HTTPClient satisfies QuiverClient.
var _ QuiverClient = (*HTTPClient)(nil)
```

- [ ] **Step 4: Run all client tests — verify everything passes**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -v -race
```

Expected: all tests pass (no specific count assertion — the compile-time `var _` line will fail loudly if any method is missing).

- [ ] **Step 5: Confirm the compile-time assertion is satisfied**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go build ./client/
```

Expected: no output (clean build). If any method is not implemented, the build error names the missing method.

- [ ] **Step 6: Commit**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core && \
git add cli/client/http.go cli/client/http_test.go && \
git commit -m "$(cat <<'EOF'
feat(cli/client): add Collection and Health methods; complete QuiverClient implementation

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Push Implementation Branch and Open PR

**Files:** none

All commands run from `/home/valen/Documents/projects/quiver/quiver-core`.

- [ ] **Step 1: Run the full test suite one final time**

```bash
cd /home/valen/Documents/projects/quiver/quiver-core/cli && go test ./client/ -race
```

Expected: `ok  quiver-cli/client`

- [ ] **Step 2: Push the implementation branch**

```bash
git push -u origin feature/cli-http-client
```

Expected: branch pushed, remote tracking set.

- [ ] **Step 3: Open the implementation PR targeting the interface branch**

Replace `<interface-pr-url>` with the URL noted in Task 2.

```bash
gh pr create \
  --base feature/cli-client \
  --head feature/cli-http-client \
  --title "feat(cli/client): HTTPClient — concrete QuiverClient over HTTP + WebSocket" \
  --body "$(cat <<'EOF'
## Summary
- Implements `HTTPClient` — the concrete `QuiverClient` for production use
- `ws.go`: `pump` function with context-cancellation and per-method terminal-state detection
- `http.go`: all 23 interface methods across Arrow catalog, runtime lifecycle, runtime observation, Collections, and Health
- `var _ QuiverClient = (*HTTPClient)(nil)` compile-time assertion in `http.go`

## Stacking
This PR targets `feature/cli-client` (interface PR). When the interface PR is accepted and merged to `develop`, this branch rebases onto `develop` and the PR retargets automatically. Review the diff — it shows only the HTTP transport layer, not the interface code.

## Test plan
- [ ] `cd cli && go test ./client/ -v -race` — all tests pass
- [ ] Verify `var _ QuiverClient = (*HTTPClient)(nil)` compiles cleanly
- [ ] Review `ws.go` terminal-state rules against `docs/superpowers/specs/2026-05-11-cli-client-interface-design.md`
- [ ] Review namespace URL encoding — all paths use `url.PathEscape(namespace)` (e.g. `github.com%2Ffoo%2Fbar`)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed.

- [ ] **Step 4: Verify both PRs are open**

```bash
gh pr list --state open
```

Expected: both `feature/cli-client` and `feature/cli-http-client` appear.
