# WebSocket State Waiting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all REST-polling `WaitForState` and `WaitForListLen` helpers with WebSocket-based event listeners that react to state changes the instant they happen, eliminating all timing dependencies from integration tests.

**Architecture:** Each `Env` subscribes to two global WebSocket streams at construction time: `/v0/arrow.runtime` for runtime state events and `/v0/arrow` for catalog events. Both watchers store event history so callers never miss events that arrived before they start waiting. Tests call `env.WaitForState` and `env.WaitForListLen` instead of the old REST-polling helpers. The `timeout` parameter remains as a safety net (not a timing window), set generously at 120s.

**Tech Stack:** `gorilla/websocket` (already in go.mod), `sync`, `encoding/json`, `internal/api/v0/dto.ArrowRuntimeDTO`, `internal/domain.ArrowState`.

---

## WebSocket Message Formats

`GET /v0/arrow.runtime` (global) pushes raw `ArrowRuntimeDTO` JSON per event:
```json
{"namespace":"quiver.test/quiver-test/tool-a@v1","state":"ready","active_run":null,"last_return":null}
```

`GET /v0/arrow` (global) pushes raw `ArrowDTO` JSON per event containing the arrow's current state including `user_installed`.

**Critical**: neither stream sends the current state on connect. Subscribe **before** triggering operations.

---

## File Map

| File | Change |
|---|---|
| `tests/integration/kit/env.go` | Add `stateWatcher`, `arrowWatcher`, `buildEnv` subscribes to both streams; `Env` gets `WaitForState` and `WaitForListLen` methods |
| `tests/integration/kit/helpers.go` | Remove REST-polling `WaitForState` and `WaitForListLen`; keep all other helpers unchanged |
| `tests/integration/lifecycle/lifecycle_test.go` | `kit.WaitForState(s.T(), tc, ns, state, t)` → `env.WaitForState(s.T(), ns, state, 120s)` (14 calls + 2 WaitForListLen) |
| `tests/integration/deps/deps_test.go` | Same substitution (19 calls) |
| `tests/integration/versioning/versioning_test.go` | Same substitution (14 calls) |
| `tests/integration/concurrency/concurrency_test.go` | Same substitution (4 calls + 1 WaitForListLen) |
| `tests/integration/stress/stress_test.go` | Same substitution (4 calls) |
| `tests/integration/edge/edge_test.go` | Same substitution (3 calls) |

---

## Task 1: Add `stateWatcher` to `kit/env.go`

**Files:**
- Modify: `tests/integration/kit/env.go`

The `stateWatcher` subscribes to `GET /v0/arrow.runtime` (global stream, no namespace), stores every event in a history slice, and lets callers wait for a specific `(namespace, state)` pair. It checks history first (catches events already received) then waits for a notification.

- [ ] **Step 1: Add stateWatcher type and constructor after the existing imports**

Read `tests/integration/kit/env.go` first. Then add to the file after the existing `Env` struct and before `BuildEnv`:

```go
// runtimeEvent mirrors the relevant fields of ArrowRuntimeDTO from the WebSocket stream.
type runtimeEvent struct {
	Namespace string `json:"namespace"`
	State     string `json:"state"`
}

// stateWatcher subscribes to GET /v0/arrow.runtime (global) and stores all events.
// WaitFor checks history first so callers never miss an event that arrived before they waited.
type stateWatcher struct {
	mu      sync.Mutex
	history []runtimeEvent
	subs    []chan struct{}
	done    chan struct{}
}

func newStateWatcher(t *testing.T, baseURL string) *stateWatcher {
	t.Helper()
	w := &stateWatcher{done: make(chan struct{})}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow.runtime"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *stateWatcher) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	for {
		select {
		case <-w.done:
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var evt runtimeEvent
		if json.Unmarshal(msg, &evt) == nil && evt.State != "" {
			w.mu.Lock()
			w.history = append(w.history, evt)
			subs := make([]chan struct{}, len(w.subs))
			copy(subs, w.subs)
			w.mu.Unlock()
			for _, ch := range subs {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
}

// WaitFor blocks until ns reaches want state or timeout elapses.
// Checks history before waiting so it never misses events that already arrived.
func (w *stateWatcher) WaitFor(
	t *testing.T,
	ns string,
	want domain.ArrowState,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	for _, evt := range w.history {
		if evt.Namespace == ns && domain.ArrowState(evt.State) == want {
			w.mu.Unlock()
			return
		}
	}
	w.subs = append(w.subs, notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		for i, ch := range w.subs {
			if ch == notify {
				w.subs = append(w.subs[:i], w.subs[i+1:]...)
				break
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		for _, evt := range w.history {
			if evt.Namespace == ns && domain.ArrowState(evt.State) == want {
				w.mu.Unlock()
				return
			}
		}
		w.mu.Unlock()

		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForState(%s): timeout waiting for %s", ns, want)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForState(%s): timeout waiting for %s", ns, want)
			return
		}
	}
}

func (w *stateWatcher) close() {
	close(w.done)
}
```

- [ ] **Step 2: Update the imports in `env.go`**

Add to the import block (if not already present):
```go
"encoding/json"
"strings"
"sync"

"github.com/gorilla/websocket"
"github.com/rabbytesoftware/quiver/internal/domain"
```

- [ ] **Step 3: Add `watcher *stateWatcher` field to `Env` struct**

Change:
```go
type Env struct {
	URL       string
	closeOnce sync.Once
	closeFn   func()
}
```
to:
```go
type Env struct {
	URL       string
	closeOnce sync.Once
	closeFn   func()
	states    *stateWatcher
}
```

- [ ] **Step 4: Create the stateWatcher in `BuildEnv` and wire it into closeAll**

In `BuildEnv`, after `srv := httptest.NewServer(apiContainer)`, add:
```go
states := newStateWatcher(t, srv.URL)
```

Change `closeFn` to also close the watcher:
```go
closeFn := func() {
	srv.Close()
	cancel()
	states.close()
	time.Sleep(500 * time.Millisecond)
}
e := &Env{URL: srv.URL, closeFn: closeFn, states: states}
```

- [ ] **Step 5: Add `WaitForState` method on `Env`**

Add after the `TypedClient` method:
```go
// WaitForState blocks until ns reaches want state or 120s elapses.
// Uses the global WebSocket stream — no polling, no timing dependency.
func (e *Env) WaitForState(t *testing.T, ns string, want domain.ArrowState, timeout time.Duration) {
	t.Helper()
	e.states.WaitFor(t, ns, want, timeout)
}
```

- [ ] **Step 6: Build to confirm compilation**

Run: `go build -tags integration ./tests/integration/...`

Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add tests/integration/kit/env.go
git commit -m "feat(integration/kit): subscribe to global runtime WebSocket for event-driven state waiting"
```

---

## Task 2: Add `arrowWatcher` for Catalog Events

**Files:**
- Modify: `tests/integration/kit/env.go`

The `arrowWatcher` subscribes to `GET /v0/arrow` (global catalog stream), counts distinct user-installed arrows, and signals when the count reaches a target. This replaces `WaitForListLen`.

- [ ] **Step 1: Add arrowEvent type and arrowWatcher**

Add after `stateWatcher`:

```go
// arrowEvent mirrors the userInstalled field from ArrowDTO pushed by GET /v0/arrow WebSocket.
type arrowEvent struct {
	Namespace     string `json:"namespace"`
	UserInstalled bool   `json:"user_installed"`
}

// arrowWatcher subscribes to GET /v0/arrow (global catalog stream).
// WaitForCount blocks until wantLen distinct user-installed bare namespaces are seen.
type arrowWatcher struct {
	mu      sync.Mutex
	seen    map[string]struct{} // bare namespaces of user-installed arrows seen so far
	subs    []chan struct{}
	done    chan struct{}
}

func newArrowWatcher(t *testing.T, baseURL string) *arrowWatcher {
	t.Helper()
	w := &arrowWatcher{
		seen: make(map[string]struct{}),
		done: make(chan struct{}),
	}
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/v0/arrow"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	go w.readLoop(conn)
	return w
}

func (w *arrowWatcher) readLoop(conn *websocket.Conn) {
	defer conn.Close()
	for {
		select {
		case <-w.done:
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var evt arrowEvent
		if json.Unmarshal(msg, &evt) == nil && evt.UserInstalled && evt.Namespace != "" {
			// Use bare namespace (strip @ref) as the dedup key
			bare := evt.Namespace
			if idx := strings.LastIndex(bare, "@"); idx >= 0 {
				bare = bare[:idx]
			}
			w.mu.Lock()
			w.seen[bare] = struct{}{}
			subs := make([]chan struct{}, len(w.subs))
			copy(subs, w.subs)
			w.mu.Unlock()
			for _, ch := range subs {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}
}

// WaitForCount blocks until wantLen distinct user-installed arrows are seen or timeout elapses.
func (w *arrowWatcher) WaitForCount(
	t *testing.T,
	wantLen int,
	timeout time.Duration,
) {
	t.Helper()
	notify := make(chan struct{}, 1)

	w.mu.Lock()
	if len(w.seen) >= wantLen {
		w.mu.Unlock()
		return
	}
	w.subs = append(w.subs, notify)
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		for i, ch := range w.subs {
			if ch == notify {
				w.subs = append(w.subs[:i], w.subs[i+1:]...)
				break
			}
		}
		w.mu.Unlock()
	}()

	deadline := time.Now().Add(timeout)
	for {
		w.mu.Lock()
		count := len(w.seen)
		w.mu.Unlock()
		if count >= wantLen {
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("WaitForListLen: timeout waiting for %d user-installed arrows (have %d)", wantLen, count)
			return
		}
		select {
		case <-notify:
		case <-time.After(remaining):
			t.Fatalf("WaitForListLen: timeout waiting for %d user-installed arrows (have %d)", wantLen, count)
			return
		}
	}
}

func (w *arrowWatcher) close() {
	close(w.done)
}
```

- [ ] **Step 2: Add `arrows *arrowWatcher` to `Env`**

```go
type Env struct {
	URL       string
	closeOnce sync.Once
	closeFn   func()
	states    *stateWatcher
	arrows    *arrowWatcher
}
```

- [ ] **Step 3: Create arrowWatcher in `BuildEnv` and wire into closeAll**

After `states := newStateWatcher(t, srv.URL)`, add:
```go
arrows := newArrowWatcher(t, srv.URL)
```

Update closeFn:
```go
closeFn := func() {
	srv.Close()
	cancel()
	states.close()
	arrows.close()
	time.Sleep(500 * time.Millisecond)
}
e := &Env{URL: srv.URL, closeFn: closeFn, states: states, arrows: arrows}
```

- [ ] **Step 4: Add `WaitForListLen` method on `Env`**

```go
// WaitForListLen blocks until wantLen distinct user-installed arrows appear in the catalog stream.
// Uses the global WebSocket catalog stream — no polling, no timing dependency.
func (e *Env) WaitForListLen(t *testing.T, wantLen int, timeout time.Duration) {
	t.Helper()
	e.arrows.WaitForCount(t, wantLen, timeout)
}
```

- [ ] **Step 5: Build**

Run: `go build -tags integration ./tests/integration/...`

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add tests/integration/kit/env.go
git commit -m "feat(integration/kit): subscribe to global arrow catalog WebSocket for event-driven list waiting"
```

---

## Task 3: Remove REST-Polling Helpers from `kit/helpers.go`

**Files:**
- Modify: `tests/integration/kit/helpers.go`

- [ ] **Step 1: Remove `WaitForState` function entirely**

Delete the entire `WaitForState` function (lines ~57-73). It polls `tc.GetDetail` every 50ms.

- [ ] **Step 2: Remove `WaitForListLen` function entirely**

Delete the entire `WaitForListLen` function (lines ~75-90). It polls `tc.List` every 50ms.

- [ ] **Step 3: Remove unused imports**

After removing both functions, `"net/http"` and `"time"` may become unused in `helpers.go`. Remove them from the import block if so. Keep `"testing"`, `"fmt"`, `dto` etc.

- [ ] **Step 4: Build**

Run: `go build -tags integration ./tests/integration/...`

Expected: Build FAILS because test files still call `kit.WaitForState` and `kit.WaitForListLen`. This is expected — Tasks 4+ fix the call sites.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/kit/helpers.go
git commit -m "refactor(integration/kit): remove REST-polling WaitForState and WaitForListLen"
```

---

## Task 4: Update Lifecycle Tests

**Files:**
- Modify: `tests/integration/lifecycle/lifecycle_test.go`

This file has 14 `kit.WaitForState` and 2 `kit.WaitForListLen` calls.

**Change pattern:**
- `kit.WaitForState(s.T(), tc, ns, state, Xs)` → `env.WaitForState(s.T(), ns, state, 120*time.Second)`
- `kit.WaitForListLen(s.T(), tc, N, Xs)` → `env.WaitForListLen(s.T(), N, 120*time.Second)`

Note: `tc` is removed from both calls. All timeout values become `120*time.Second` (generous safety net, not a timing dependency).

- [ ] **Step 1: Apply substitutions**

```bash
# In lifecycle_test.go:
# Replace WaitForState: remove tc argument, set timeout to 120s
sed -i '' 's/kit\.WaitForState(s\.T(), tc, /env.WaitForState(s.T(), /g' \
    tests/integration/lifecycle/lifecycle_test.go

# Replace timeout values (any Xs or Xm) with 120*time.Second
sed -i '' 's/, [0-9]*\*time\.[A-Za-z]*)$/,\ 120*time.Second)/g' \
    tests/integration/lifecycle/lifecycle_test.go

# Replace WaitForListLen: remove tc argument, set timeout to 120s
sed -i '' 's/kit\.WaitForListLen(s\.T(), tc, /env.WaitForListLen(s.T(), /g' \
    tests/integration/lifecycle/lifecycle_test.go
```

After sed, verify manually that calls look correct. Example before/after:
```go
// Before:
kit.WaitForState(s.T(), tc, kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 15*time.Second)
items := kit.WaitForListLen(s.T(), tc, 1, 5*time.Second)

// After:
env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
items := env.WaitForListLen(s.T(), 1, 120*time.Second)
```

Note: `WaitForListLen` previously returned `[]dto.ArrowListItemDTO`. The new `env.WaitForListLen` returns nothing (only waits). If the test needs the list items after waiting, follow up with `tc.List()`.

- [ ] **Step 2: Fix WaitForListLen return value usages**

In lifecycle_test.go, `kit.WaitForListLen` returns items. Find usages like:
```go
items := kit.WaitForListLen(s.T(), tc, 1, 5*time.Second)
s.Len(items, 1, "...")
```

Replace with:
```go
env.WaitForListLen(s.T(), 1, 120*time.Second)
items, _ := tc.List()
s.Len(items, 1, "...")
```

- [ ] **Step 3: Build**

Run: `go build -tags integration ./tests/integration/lifecycle/...`

Expected: no errors.

- [ ] **Step 4: Run lifecycle tests locally**

```bash
go test -tags integration -race -timeout 120s ./tests/integration/lifecycle/... -v 2>&1 | grep -E "PASS|FAIL"
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add tests/integration/lifecycle/lifecycle_test.go
git commit -m "refactor(integration/lifecycle): WaitForState → env.WaitForState (WebSocket-based)"
```

---

## Task 5: Update Deps Tests

**Files:**
- Modify: `tests/integration/deps/deps_test.go`

19 `kit.WaitForState` calls.

- [ ] **Step 1: Apply substitutions**

```bash
sed -i '' 's/kit\.WaitForState(s\.T(), tc, /env.WaitForState(s.T(), /g' \
    tests/integration/deps/deps_test.go
sed -i '' 's/, [0-9]*\*time\.[A-Za-z]*)$/,\ 120*time.Second)/g' \
    tests/integration/deps/deps_test.go
```

Verify manually that 19 calls are updated.

- [ ] **Step 2: Build and run**

```bash
go build -tags integration ./tests/integration/deps/...
go test -tags integration -race -timeout 120s ./tests/integration/deps/... -v 2>&1 | grep -E "PASS|FAIL"
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add tests/integration/deps/deps_test.go
git commit -m "refactor(integration/deps): WaitForState → env.WaitForState (WebSocket-based)"
```

---

## Task 6: Update Versioning Tests

**Files:**
- Modify: `tests/integration/versioning/versioning_test.go`

14 calls.

- [ ] **Step 1: Apply substitutions**

```bash
sed -i '' 's/kit\.WaitForState(s\.T(), tc, /env.WaitForState(s.T(), /g' \
    tests/integration/versioning/versioning_test.go
sed -i '' 's/, [0-9]*\*time\.[A-Za-z]*)$/,\ 120*time.Second)/g' \
    tests/integration/versioning/versioning_test.go
```

- [ ] **Step 2: Build and run**

```bash
go build -tags integration ./tests/integration/versioning/...
go test -tags integration -race -timeout 120s ./tests/integration/versioning/... -v 2>&1 | grep -E "PASS|FAIL"
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add tests/integration/versioning/versioning_test.go
git commit -m "refactor(integration/versioning): WaitForState → env.WaitForState (WebSocket-based)"
```

---

## Task 7: Update Concurrency, Stress, and Edge Tests

**Files:**
- Modify: `tests/integration/concurrency/concurrency_test.go` (4 + 1 WaitForListLen)
- Modify: `tests/integration/stress/stress_test.go` (4)
- Modify: `tests/integration/edge/edge_test.go` (3)

- [ ] **Step 1: Apply substitutions to all three files**

```bash
for f in tests/integration/concurrency/concurrency_test.go \
          tests/integration/stress/stress_test.go \
          tests/integration/edge/edge_test.go; do
    sed -i '' 's/kit\.WaitForState(s\.T(), tc, /env.WaitForState(s.T(), /g' "$f"
    sed -i '' 's/kit\.WaitForListLen(s\.T(), tc, /env.WaitForListLen(s.T(), /g' "$f"
    sed -i '' 's/, [0-9]*\*time\.[A-Za-z]*)$/,\ 120*time.Second)/g' "$f"
done
```

Fix any `WaitForListLen` return value usages in concurrency_test.go:
```go
// Before:
items := kit.WaitForListLen(s.T(), tc, 1, 10*time.Second)
s.Len(items, 1, "...")

// After:
env.WaitForListLen(s.T(), 1, 120*time.Second)
items, _ := tc.List()
s.Len(items, 1, "...")
```

- [ ] **Step 2: Build all three**

```bash
go build -tags integration \
    ./tests/integration/concurrency/... \
    ./tests/integration/stress/... \
    ./tests/integration/edge/...
```

Expected: no errors.

- [ ] **Step 3: Run all three**

```bash
go test -tags integration -race -timeout 120s \
    ./tests/integration/concurrency/... \
    ./tests/integration/stress/... \
    ./tests/integration/edge/... \
    -v 2>&1 | grep -E "PASS|FAIL"
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add tests/integration/concurrency/concurrency_test.go \
        tests/integration/stress/stress_test.go \
        tests/integration/edge/edge_test.go
git commit -m "refactor(integration): WaitForState → env.WaitForState across concurrency/stress/edge suites"
```

---

## Task 8: Run Full Suite and Verify

- [ ] **Step 1: Run the full integration suite locally**

```bash
go test -tags integration -race -timeout 300s ./tests/integration/... 2>&1 | grep -E "^ok|^FAIL"
```

Expected:
```
ok      github.com/rabbytesoftware/quiver/tests/integration/concurrency
ok      github.com/rabbytesoftware/quiver/tests/integration/deps
ok      github.com/rabbytesoftware/quiver/tests/integration/edge
ok      github.com/rabbytesoftware/quiver/tests/integration/lifecycle
ok      github.com/rabbytesoftware/quiver/tests/integration/stress
ok      github.com/rabbytesoftware/quiver/tests/integration/versioning
```

All 6 suites PASS.

- [ ] **Step 2: Verify no remaining `kit.WaitForState` calls in test files**

```bash
grep -rn "kit\.WaitForState\|kit\.WaitForListLen" tests/integration/ --include="*.go" | grep -v "^tests/integration/kit/"
```

Expected: empty output.

- [ ] **Step 3: Commit final cleanup**

```bash
git add -A
git commit -m "test(integration): all state waiting is now WebSocket-driven, no REST polling"
```

---

## Task 9: Push and Monitor CI

- [ ] **Step 1: Push**

```bash
git push
```

- [ ] **Step 2: Monitor CI**

```bash
gh run list --branch enhancement/arrow-v0-better-multi-os --limit 2
```

Watch the Integration Tests job. It should pass reliably now because:
- `WaitForState` reacts instantly to events — no 30s polling windows to miss
- No goroutine accumulation causing starvation (WebSocket event delivery is independent of asynx worker scheduling)
- Safety-net timeout is 120s — far more generous than the 15–30s that was relied upon before

---

## Self-Review

**Spec coverage:**
- ✅ Replace all `kit.WaitForState` (REST polling) with WebSocket → Tasks 1, 4-7
- ✅ Replace all `kit.WaitForListLen` (REST polling) with WebSocket → Tasks 2, 4, 7
- ✅ Remove REST-polling functions from kit/helpers.go → Task 3
- ✅ Verify full suite passes locally → Task 8
- ✅ Push to CI → Task 9

**Placeholder scan:** No TBDs, no "handle this later", no missing implementations.

**Type consistency:**
- `env.WaitForState(t *testing.T, ns string, want domain.ArrowState, timeout time.Duration)` — consistent across all tasks
- `env.WaitForListLen(t *testing.T, wantLen int, timeout time.Duration)` — consistent across all tasks
- `stateWatcher.WaitFor` and `arrowWatcher.WaitForCount` are internal — not exposed in call sites
