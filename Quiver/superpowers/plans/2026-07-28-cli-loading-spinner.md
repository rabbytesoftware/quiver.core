# CLI General Loading Spinner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show an indeterminate loading spinner for non-lifecycle CLI commands and the daemon auto-boot wait, so blocking operations give feedback instead of hanging silently.

**Architecture:** A lightweight carriage-return spinner primitive in `internal/cli/ui` that renders only after a delay (flicker-free for fast ops). Commands opt in via a TTY-gated `(a *app) withSpinner` helper; `session()` wraps daemon auto-boot; each non-lifecycle command wraps its client call. Lifecycle commands keep their existing BubbleTea progress view.

**Tech Stack:** Go, cobra, lipgloss (already used by `internal/cli/ui`). No new dependencies.

## Global Constraints

- Spinner is **TTY-only**: when `a.deps.IsTTY()` is false (piped / `-o json|yaml`), `withSpinner` runs the function with zero output.
- Spinner writes to **stderr** (`cmd.ErrOrStderr()`); result output stays on stdout, unchanged.
- Delay constant: `spinnerDelay = 120 * time.Millisecond`. Tick interval: `100 * time.Millisecond`.
- Braille frames live once in `ui.SpinnerFrames`; the lifecycle view reuses them.
- **Cancellation:** current behavior is unchanged — `cmd/quiver/main.go` uses `root.Execute()` (background context), so Ctrl-C terminates the process as it does today. We deliberately do NOT add global `signal.NotifyContext` wiring: it would collide with BubbleTea's own SIGINT handling in the lifecycle view. The spinner is best-effort; it clears on normal completion or error.
- Follow existing patterns: error wrapping, `gofumpt`/`goimports` via `make fmt`, table-driven tests with `testify`, `TestType_Method_Desc` naming.
- Run Go via `export PATH=/usr/local/go/bin:$PATH`. Tests: `go test -race ./...`.

---

### Task 1: Spinner primitive in `internal/cli/ui`

**Files:**
- Create: `internal/cli/ui/spinner.go`
- Create: `internal/cli/ui/spinner_test.go`
- Modify: `internal/cli/lifecycle/model.go:21` (reuse shared frames)

**Interfaces:**
- Produces:
  - `ui.SpinnerFrames []string` — the 8 braille frames.
  - `ui.NewSpinner(w io.Writer, label string, delay time.Duration) *ui.Spinner`
  - `(*ui.Spinner).Start()` — launches the render goroutine; returns immediately; writes nothing until `delay` elapses.
  - `(*ui.Spinner).Stop()` — stops the goroutine and clears the line if anything was drawn; idempotent; must be called after `Start`.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/ui/spinner_test.go`:

```go
package ui_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

// syncBuffer is a concurrency-safe writer for asserting spinner output.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestSpinner_FastPathWritesNothing(t *testing.T) {
	buf := &syncBuffer{}
	sp := ui.NewSpinner(buf, "loading", 50*time.Millisecond)
	sp.Start()
	sp.Stop() // stops before the 50ms delay elapses
	assert.Empty(t, buf.String(), "an op that finishes before the delay must render nothing")
}

func TestSpinner_DrawsLabelThenClears(t *testing.T) {
	buf := &syncBuffer{}
	sp := ui.NewSpinner(buf, "loading", 1*time.Millisecond)
	sp.Start()
	assert.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "loading")
	}, time.Second, 5*time.Millisecond, "spinner should draw the label after the delay")
	sp.Stop()
	assert.True(t, strings.HasSuffix(buf.String(), "\r\033[K"),
		"Stop must clear the line after drawing")
}

func TestSpinner_StopIsIdempotent(t *testing.T) {
	buf := &syncBuffer{}
	sp := ui.NewSpinner(buf, "x", 1*time.Millisecond)
	sp.Start()
	sp.Stop()
	sp.Stop() // must not panic
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `export PATH=/usr/local/go/bin:$PATH && go test ./internal/cli/ui/ -run TestSpinner`
Expected: FAIL — `undefined: ui.NewSpinner` / `ui.Spinner`.

- [ ] **Step 3: Implement the primitive**

Create `internal/cli/ui/spinner.go`:

```go
package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// SpinnerFrames are the braille frames shared by the spinner and the lifecycle
// progress view.
var SpinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Spinner renders an indeterminate single-line spinner to w, but only after
// delay elapses, so operations that finish quickly render nothing. It writes a
// carriage return before each frame and clears the line on Stop.
type Spinner struct {
	w     io.Writer
	label string
	delay time.Duration

	stop chan struct{}
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	printed bool
}

// NewSpinner builds a spinner that writes to w and starts drawing after delay.
func NewSpinner(w io.Writer, label string, delay time.Duration) *Spinner {
	return &Spinner{
		w:     w,
		label: label,
		delay: delay,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Start launches the render loop. It returns immediately; nothing is written
// until delay passes.
func (s *Spinner) Start() { go s.run() }

func (s *Spinner) run() {
	defer close(s.done)

	select {
	case <-time.After(s.delay):
	case <-s.stop:
		return // stopped before the delay: never drew anything
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frame := 0
	s.draw(frame)
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			frame++
			s.draw(frame)
		}
	}
}

func (s *Spinner) draw(frame int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.printed = true
	_, _ = fmt.Fprintf(s.w, "\r%s %s",
		Brand.Render(SpinnerFrames[frame%len(SpinnerFrames)]),
		Muted.Render(s.label))
}

// Stop halts the spinner and clears its line if anything was drawn. It is safe
// to call more than once, and must be called after Start.
func (s *Spinner) Stop() {
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.printed {
			_, _ = fmt.Fprint(s.w, "\r\033[K") // carriage return + clear to end of line
		}
	})
}
```

- [ ] **Step 4: Reuse the shared frames in the lifecycle view**

In `internal/cli/lifecycle/model.go`, replace the local frames declaration (line 21):

```go
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
```

with a reference to the shared slice (the file already imports `internal/cli/ui`):

```go
var spinnerFrames = ui.SpinnerFrames
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `export PATH=/usr/local/go/bin:$PATH && go test -race ./internal/cli/ui/ ./internal/cli/lifecycle/`
Expected: PASS (spinner tests + unchanged lifecycle tests).

- [ ] **Step 6: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/ui/spinner.go internal/cli/ui/spinner_test.go internal/cli/lifecycle/model.go
git commit -m "feat(cli): add delayed indeterminate spinner primitive"
```

---

### Task 2: `withSpinner` helper + daemon-boot wiring

**Files:**
- Modify: `internal/cli/commands/commands.go` (add helper; wrap `EnsureDaemon` in `session`)
- Create: `internal/cli/commands/spinner_internal_test.go`

**Interfaces:**
- Consumes: `ui.NewSpinner`, `(*ui.Spinner).Start/Stop` from Task 1.
- Produces:
  - `const spinnerDelay = 120 * time.Millisecond` (package `commands`)
  - `(a *app) withSpinner(cmd *cobra.Command, label string, fn func() error) error` — runs `fn`; on a TTY, shows a delayed spinner on stderr around it; returns `fn`'s error.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/commands/spinner_internal_test.go`:

```go
package commands

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSpinnerTestCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	cmd.SetOut(&bytes.Buffer{})
	return cmd, &errBuf
}

func TestWithSpinner_NonTTYRunsSilently(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}
	cmd, errBuf := newSpinnerTestCmd()

	ran := false
	err := a.withSpinner(cmd, "loading", func() error { ran = true; return nil })

	require.NoError(t, err)
	assert.True(t, ran, "fn must run")
	assert.Empty(t, errBuf.String(), "no spinner output when not a TTY")
}

func TestWithSpinner_PropagatesError(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}
	cmd, _ := newSpinnerTestCmd()

	sentinel := errors.New("boom")
	err := a.withSpinner(cmd, "loading", func() error { return sentinel })

	assert.ErrorIs(t, err, sentinel)
}

func TestWithSpinner_FastTTYOpDoesNotFlash(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return true }}}
	cmd, errBuf := newSpinnerTestCmd()

	// fn returns immediately, well under spinnerDelay → nothing is drawn.
	err := a.withSpinner(cmd, "loading", func() error { return nil })

	require.NoError(t, err)
	assert.Empty(t, errBuf.String(), "a fast op must not flash the spinner")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `export PATH=/usr/local/go/bin:$PATH && go test ./internal/cli/commands/ -run TestWithSpinner`
Expected: FAIL — `a.withSpinner undefined`.

- [ ] **Step 3: Implement the helper**

In `internal/cli/commands/commands.go`, add the import `"time"` and the constant + method (place near `session`):

```go
const spinnerDelay = 120 * time.Millisecond

// withSpinner runs fn, showing a delayed loading spinner on stderr while it
// blocks. On a non-interactive stdout it just runs fn with no output.
func (a *app) withSpinner(cmd *cobra.Command, label string, fn func() error) error {
	if !a.deps.IsTTY() {
		return fn()
	}
	sp := ui.NewSpinner(cmd.ErrOrStderr(), label, spinnerDelay)
	sp.Start()
	defer sp.Stop()
	return fn()
}
```

Ensure `internal/cli/ui` is imported in `commands.go` (it already is via other files in the package, but this file needs its own import).

- [ ] **Step 4: Wrap daemon auto-boot in `session`**

In `internal/cli/commands/commands.go`, change the `EnsureDaemon` call inside `session`:

```go
if strings.HasPrefix(server, "unix://") && a.deps.EnsureDaemon != nil {
	if err := a.deps.EnsureDaemon(cmd.Context()); err != nil {
		return nil, err
	}
}
```

to:

```go
if strings.HasPrefix(server, "unix://") && a.deps.EnsureDaemon != nil {
	if err := a.withSpinner(cmd, "starting daemon", func() error {
		return a.deps.EnsureDaemon(cmd.Context())
	}); err != nil {
		return nil, err
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `export PATH=/usr/local/go/bin:$PATH && go test -race ./internal/cli/commands/`
Expected: PASS (new spinner tests + all existing command tests, which use `IsTTY=false`).

- [ ] **Step 6: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/commands/commands.go internal/cli/commands/spinner_internal_test.go
git commit -m "feat(cli): withSpinner helper + spinner on daemon auto-boot"
```

---

### Task 3: Wrap mutation commands

**Files:**
- Modify: `internal/cli/commands/arrow.go` (add/remove/refresh)
- Modify: `internal/cli/commands/collection.go` (follow/unfollow/update)

**Interfaces:**
- Consumes: `(a *app) withSpinner` from Task 2.

Wrapping rule (applies to every edit below): call `a.session(cmd)` FIRST (its own "starting daemon" spinner runs there), then wrap ONLY the client call in `withSpinner` — never nest a `withSpinner` around `session`.

- [ ] **Step 1: Wrap `arrow refresh` / `add` / `remove`**

In `internal/cli/commands/arrow.go`:

`arrowRefreshCmd` — replace `if err := cli.RefreshArrow(cmd.Context(), args[0]); err != nil {`:

```go
if err := a.withSpinner(cmd, "refreshing "+args[0], func() error {
	return cli.RefreshArrow(cmd.Context(), args[0])
}); err != nil {
	return err
}
```

`arrowAddCmd` — replace `if err := cli.AddArrow(cmd.Context(), args[0]); err != nil {`:

```go
if err := a.withSpinner(cmd, "adding "+args[0], func() error {
	return cli.AddArrow(cmd.Context(), args[0])
}); err != nil {
	return err
}
```

`arrowRemoveCmd` — replace `if err := cli.RemoveArrow(cmd.Context(), args[0]); err != nil {`:

```go
if err := a.withSpinner(cmd, "removing "+args[0], func() error {
	return cli.RemoveArrow(cmd.Context(), args[0])
}); err != nil {
	return err
}
```

- [ ] **Step 2: Wrap collection follow/unfollow/update**

In `internal/cli/commands/collection.go`, each of `collectionFollowCmd`, `collectionUnfollowCmd`, `collectionUpdateCmd` has a `call` closure of the shape `cli, err := a.session(cmd); ...; return cli.XxxCollection(cmd.Context(), ns)`. Wrap the final client call:

Follow:
```go
return a.withSpinner(cmd, "following "+ns, func() error {
	return cli.FollowCollection(cmd.Context(), ns)
})
```
Unfollow:
```go
return a.withSpinner(cmd, "unfollowing "+ns, func() error {
	return cli.UnfollowCollection(cmd.Context(), ns)
})
```
Update:
```go
return a.withSpinner(cmd, "updating "+ns, func() error {
	return cli.UpdateCollection(cmd.Context(), ns)
})
```

- [ ] **Step 3: Run the command suite (deliverable: green, non-TTY behavior unchanged)**

Run: `export PATH=/usr/local/go/bin:$PATH && go test -race ./internal/cli/commands/`
Expected: PASS — existing add/remove/refresh/follow tests (all `IsTTY=false`) still produce the same `added/removed/refreshed/following` output; `withSpinner` is a passthrough there.

- [ ] **Step 4: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/commands/arrow.go internal/cli/commands/collection.go
git commit -m "feat(cli): loading spinner on catalog mutation commands"
```

---

### Task 4: Wrap query commands

**Files:**
- Modify: `internal/cli/commands/discovery.go` (`fetchCatalog` for list/search; `info`; `methods`)
- Modify: `internal/cli/commands/observe.go` (`status`, `ps`)
- Modify: `internal/cli/commands/arrow.go` (`arrow list`, `arrow show`)
- Modify: `internal/cli/commands/collection.go` (`collection list`, `collection show`)

**Interfaces:**
- Consumes: `(a *app) withSpinner` from Task 2.

Same rule: `session` first, then wrap the fetch call(s) only. Label is `"loading"` for all queries.

- [ ] **Step 1: Wrap `fetchCatalog` (covers `list` and `search`)**

In `internal/cli/commands/discovery.go`, `fetchCatalog` currently does, after `session`:

```go
arrows, err := cli.ListArrows(cmd.Context(), nil)
if err != nil {
	return catalogDoc{}, err
}
collections, err := cli.ListCollections(cmd.Context())
if err != nil {
	return catalogDoc{}, err
}
```

Wrap both fetches in one spinner:

```go
var (
	arrows      []apidto.ArrowListItemDTO
	collections []apidto.CollectionListItemDTO
)
if err := a.withSpinner(cmd, "loading", func() error {
	var e error
	arrows, e = cli.ListArrows(cmd.Context(), nil)
	if e != nil {
		return e
	}
	collections, e = cli.ListCollections(cmd.Context())
	return e
}); err != nil {
	return catalogDoc{}, err
}
```

(Keep the rest of `fetchCatalog` — the `doc` build and filtering — unchanged.)

- [ ] **Step 2: Wrap `info` and `methods` fetches**

In `internal/cli/commands/discovery.go`:

`infoCmd` manifest branch — replace `raw, err := cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))`:
```go
var raw json.RawMessage
if err := a.withSpinner(cmd, "loading", func() error {
	var e error
	raw, e = cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))
	return e
}); err != nil {
	return err
}
```
`infoCmd` detail branch — replace `detail, err := cli.GetArrow(cmd.Context(), args[0])`:
```go
var detail apidto.ArrowDetailDTO
if err := a.withSpinner(cmd, "loading", func() error {
	var e error
	detail, e = cli.GetArrow(cmd.Context(), args[0])
	return e
}); err != nil {
	return err
}
```
`methodsCmd` — replace `raw, err := cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))` with the same `withSpinner(cmd, "loading", …)` wrapper assigning `raw`.

- [ ] **Step 3: Wrap `status` and `ps` fetches**

In `internal/cli/commands/observe.go`, wrap each `cli.ListRuntimes(cmd.Context())` and `cli.GetRuntime(cmd.Context(), args[0])` in `a.withSpinner(cmd, "loading", func() error { … })`, assigning the result into a var declared just above (same pattern as Step 2).

- [ ] **Step 4: Wrap `arrow list` / `arrow show` and `collection list` / `collection show`**

In `internal/cli/commands/arrow.go` (`arrowListCmd` → `cli.ListArrows`, `arrowShowCmd` → `cli.GetArrow`) and `internal/cli/commands/collection.go` (`collectionListCmd` → `cli.ListCollections`, `collectionShowCmd` → `cli.GetCollection`), wrap each fetch in `a.withSpinner(cmd, "loading", func() error { … })` assigning into a var declared above.

- [ ] **Step 5: Run the full suite (deliverable: green)**

Run: `export PATH=/usr/local/go/bin:$PATH && go test -race ./...`
Expected: PASS — all existing query tests (`IsTTY=false`) render identical output.

- [ ] **Step 6: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/commands/discovery.go internal/cli/commands/observe.go internal/cli/commands/arrow.go internal/cli/commands/collection.go
git commit -m "feat(cli): loading spinner on query commands"
```

---

## Manual verification (after Task 4)

On an interactive terminal, against a stopped daemon:

```bash
export PATH=/usr/local/go/bin:$PATH
go build -o bin/quiver ./cmd/quiver
./bin/quiver arrow add github.com/Valentin-Vi/quiver.arrow-discord   # expect: "⣾ starting daemon" then "⣾ adding …", then "added …"
./bin/quiver list                                                    # fast local read → no spinner flash
./bin/quiver arrow refresh github.com/Valentin-Vi/quiver.arrow-discord  # expect "⣾ refreshing …" during the GitHub fetch
./bin/quiver arrow add github.com/Valentin-Vi/quiver.arrow-discord | cat # piped → no spinner in output
```

---

### Task 5: Rename confirmation flag `--force` → `--yes` (bug #2)

**Goal:** Clean CLI — the confirmation-skip flag is named `--yes` (shorthand `-y`) and exists ONLY on commands that prompt (`uninstall`, `arrow remove`, `collection unfollow`). Non-prompting commands keep no such flag.

**Files:**
- Modify: `internal/cli/commands/lifecycle.go` (`methodOpts.force`→`yes`; flag; confirm call)
- Modify: `internal/cli/commands/arrow.go` (`arrowRemoveCmd` local var + flag + confirm call)
- Modify: `internal/cli/commands/collection.go` (`collectionUnfollowCmd` var + flag + confirm call)
- Modify: `internal/cli/commands/commands.go` (`confirm` message)
- Modify: `internal/cli/commands/commands_test.go` (three `--force` usages → `--yes`)

- [ ] **Step 1: Update the tests to the new flag (they fail first)**

In `internal/cli/commands/commands_test.go`, change the three `"--force"` arguments to `"--yes"`:
- line ~230 (`TestUninstall_ForceSkipsConfirmation`) — also rename the test to `TestUninstall_YesFlagSkipsConfirmation`.
- line ~439 (arrow remove `--force`) — rename that test's `--force` to `--yes`.
- line ~473 (collection unfollow `--force`) — change to `--yes`.

- [ ] **Step 2: Run to verify failure**

Run: `export PATH=/usr/local/go/bin:$PATH && go test ./internal/cli/commands/ -run 'Uninstall|Remove|Unfollow'`
Expected: FAIL — `unknown flag: --yes`.

- [ ] **Step 3: Rename the flag in production**

`internal/cli/commands/lifecycle.go`:
- In `methodOpts`, rename field `force bool` → `yes bool`.
- Confirm call (line ~30): `a.confirm(cmd, opts.force, op+" "+args[0])` → `a.confirm(cmd, opts.yes, op+" "+args[0])`.
- Flag (line ~40): `cmd.Flags().BoolVarP(&opts.force, "force", "y", false, "skip the confirmation prompt")` → `cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "skip the confirmation prompt")`.

`internal/cli/commands/arrow.go` (`arrowRemoveCmd`):
- `var force bool` → `var yes bool`
- `a.confirm(cmd, force, "remove "+args[0])` → `a.confirm(cmd, yes, "remove "+args[0])`
- `cmd.Flags().BoolVarP(&force, "force", "y", false, "skip the confirmation prompt")` → `cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")`

`internal/cli/commands/collection.go` (`collectionUnfollowCmd`):
- `var force bool` → `var yes bool`
- `a.confirm(cmd, force, "unfollow "+ns)` → `a.confirm(cmd, yes, "unfollow "+ns)`
- `cmd.Flags().BoolVar(&force, "force", false, "skip the confirmation prompt")` → `cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")` (note: BoolVar→BoolVarP to add the `-y` shorthand for consistency).

`internal/cli/commands/commands.go` (`confirm`):
- Rename the parameter `force bool` → `yes bool` and update its use.
- Message: `"%s requires --force when not running interactively"` (or `--force/-y`) → `"%s requires --yes/-y when not running interactively"`.

- [ ] **Step 4: Run to verify pass**

Run: `export PATH=/usr/local/go/bin:$PATH && go test -race ./internal/cli/commands/`
Expected: PASS (the `-y` shorthand tests still pass; `--yes` now recognized; no `--force` remains).

- [ ] **Step 5: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/commands/lifecycle.go internal/cli/commands/arrow.go internal/cli/commands/collection.go internal/cli/commands/commands.go internal/cli/commands/commands_test.go
git commit -m "fix(cli): rename confirmation flag --force to --yes"
```

---

### Task 6: Skip idle-stop after lifecycle-method commands (bug #1)

**Goal:** A Ctrl-C'd install no longer leaves the arrow stuck in `installing`. The CLI must not idle-stop the daemon right after a lifecycle-method command — the daemon (a `setsid` process, unaffected by terminal Ctrl-C) then finishes the install on its own. It gets reaped later by the next read/catalog command once genuinely idle.

**Approach:** Mark lifecycle commands with a cobra annotation; in `main`, use `root.ExecuteC()` (returns the command that actually ran) and skip `stopIdleDaemon` when that command is annotated. This is robust to global flags that take values (`--server X`, `-o json`), unlike raw arg-sniffing.

**Files:**
- Modify: `internal/cli/commands/lifecycle.go` (annotate lifecycle commands)
- Create: `internal/cli/commands/lifecycle_annotation.go` (const + `IsLifecycle` helper) — or add to `commands.go`
- Modify: `cmd/quiver/main.go` (`ExecuteC` + guard)
- Test: `internal/cli/commands/lifecycle_internal_test.go` (package `commands`)

**Interfaces produced:**
- `commands.AnnotationLifecycle` (string const)
- `commands.IsLifecycle(cmd *cobra.Command) bool`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/commands/lifecycle_internal_test.go`:

```go
package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestIsLifecycle(t *testing.T) {
	a := &app{deps: Deps{IsTTY: func() bool { return false }}}

	if !IsLifecycle(a.installCmd()) {
		t.Error("install must be annotated lifecycle")
	}
	if !IsLifecycle(a.uninstallCmd()) {
		t.Error("uninstall must be annotated lifecycle")
	}
	if IsLifecycle(a.arrowCmd()) {
		t.Error("arrow (catalog) must NOT be lifecycle")
	}
	if IsLifecycle(nil) {
		t.Error("nil command is not lifecycle")
	}
	if IsLifecycle(&cobra.Command{}) {
		t.Error("un-annotated command is not lifecycle")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `export PATH=/usr/local/go/bin:$PATH && go test ./internal/cli/commands/ -run TestIsLifecycle`
Expected: FAIL — `undefined: IsLifecycle` / `AnnotationLifecycle`.

- [ ] **Step 3: Implement the annotation + helper**

Add to `internal/cli/commands/commands.go` (or a new `lifecycle_annotation.go`):

```go
// AnnotationLifecycle marks a command that starts runtime work (install, run,
// stop, update, uninstall). The daemon must not be idle-stopped right after
// one of these, since it may still be executing.
const AnnotationLifecycle = "quiver_lifecycle"

// IsLifecycle reports whether cmd is a lifecycle-method command.
func IsLifecycle(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[AnnotationLifecycle] == "true"
}
```

In `internal/cli/commands/lifecycle.go`, in `lifecycleCmd`, set the annotation on the built command (add after the `cmd := &cobra.Command{...}` literal):

```go
cmd.Annotations = map[string]string{AnnotationLifecycle: "true"}
```

- [ ] **Step 4: Run to verify pass**

Run: `export PATH=/usr/local/go/bin:$PATH && go test ./internal/cli/commands/ -run TestIsLifecycle`
Expected: PASS.

- [ ] **Step 5: Wire into main's exit path**

In `cmd/quiver/main.go`, change:
```go
err := root.Execute()

if shouldManageDaemon(os.Args[1:]) {
	if mgr, mgrErr := daemon.NewManager(); mgrErr == nil {
		stopIdleDaemon(context.Background(), mgr)
	}
}
```
to:
```go
executed, err := root.ExecuteC()

if shouldManageDaemon(os.Args[1:]) && !commands.IsLifecycle(executed) {
	if mgr, mgrErr := daemon.NewManager(); mgrErr == nil {
		stopIdleDaemon(context.Background(), mgr)
	}
}
```
(`main.go` already imports the `commands` package.)

- [ ] **Step 6: Run tests + build**

Run: `export PATH=/usr/local/go/bin:$PATH && go build ./... && go test -race ./internal/cli/... ./cmd/quiver/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
export PATH=/usr/local/go/bin:$PATH
make fmt
git add internal/cli/commands/lifecycle.go internal/cli/commands/commands.go internal/cli/commands/lifecycle_internal_test.go cmd/quiver/main.go
git commit -m "fix(cli): don't idle-stop the daemon after lifecycle commands"
```
