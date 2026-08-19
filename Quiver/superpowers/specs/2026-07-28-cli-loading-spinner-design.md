# CLI general loading spinner — design

## Problem

Lifecycle commands (`install`, `uninstall`, `run`, `stop`, `update`) render a rich
BubbleTea progress view. Every other command is a silent blocking
request/response: it calls the client, waits, then prints a one-line result. When
that wait is real — `arrow add` / `arrow refresh` / `collection follow` fetch a
manifest from GitHub (seconds), and any command on a cold start blocks on daemon
auto-boot (1–2s) — the terminal simply hangs with no feedback.

Add a **general, indeterminate loading spinner** for commands that have no
progress view of their own, plus the daemon auto-boot wait.

## Scope

- **In:** every non-lifecycle command's blocking server call (mutations and
  queries) and the daemon auto-boot wait.
- **Out:** lifecycle commands (keep their BubbleTea progress view); purely local
  work with no server round-trip; non-TTY output; percentage/determinate
  progress.

## Approach

A lightweight carriage-return spinner primitive that blocking commands opt into —
**not** BubbleTea. BubbleTea is a full event-loop program, overkill for a
one-line "working…" indicator and awkward to compose with the existing `Fprintf`
result lines. A `\r`-based spinner has no alt-screen concerns and is trivially
testable.

Rejected alternatives:
- Wrap each command's whole `RunE` generically — too coarse; spins during local
  arg parsing and would fight the lifecycle view.
- Put the spinner in the `client` package — wrong layer; the HTTP client must not
  know about TTY/UI.

## Components

### 1. `internal/cli/ui/spinner.go` — the primitive

```go
type Spinner struct { … }

// NewSpinner renders an indeterminate spinner to w, starting only after delay.
func NewSpinner(w io.Writer, label string, delay time.Duration) *Spinner
func (s *Spinner) Start() // goroutine: after delay, tick braille frames "⣾ label"
func (s *Spinner) Stop()  // stop the goroutine and erase the line (\r + clear-to-EOL)
```

- Reuses the existing braille frames and `ui.Brand` color for consistency with
  the lifecycle spinner.
- Nothing is written before `delay` elapses — this is the "delayed" behavior that
  keeps fast ops flicker-free.
- `Stop` is idempotent and safe to call after a never-started (fast) run.

### 2. `internal/cli/commands` — the `withSpinner` helper

```go
const spinnerDelay = 120 * time.Millisecond

func (a *app) withSpinner(cmd *cobra.Command, label string, fn func() error) error {
    if !a.deps.IsTTY() {
        return fn() // piped / -o json|yaml: silent
    }
    sp := ui.NewSpinner(cmd.ErrOrStderr(), label, spinnerDelay)
    sp.Start()
    defer sp.Stop()
    return fn()
}
```

- Spinner writes to **stderr** so stdout stays clean; the deferred `Stop` erases
  the line before the command prints its result to stdout.
- TTY gating lives here (via `a.deps.IsTTY()`); the primitive stays UI-only.

## Wiring

- **Daemon boot:** `session()` wraps its `EnsureDaemon` call in
  `withSpinner(cmd, "starting daemon", …)`. Because it is delayed, an
  already-running daemon shows nothing; a cold boot shows it. This covers every
  command automatically without per-command changes.
- **Blocking calls:** each non-lifecycle command wraps its single client call:
  - mutations: `adding <ns>`, `removing <ns>`, `refreshing <ns>`,
    `following <ns>`, `unfollowing <ns>`, `updating <ns>`.
  - queries (`list`, `info`, `status`, `search`, `ps`, `methods`): `loading`.
  The result-printing / `render()` stays after the wrapped call, unchanged.
- **Lifecycle commands:** untouched.

## Cancellation & behavior

- Ctrl-C cancels via `cmd.Context()` → the in-flight client call unwinds → the
  wrapped `fn` returns → deferred `Stop` clears the line.
- Verify the root command wires SIGINT → context cancellation; add it if missing.
- Exit codes and result output are unchanged.

## Testing

- `ui.Spinner`:
  - with a tiny delay and a controllable stop, assert frames are written then the
    line is cleared;
  - with a delay longer than the run, assert nothing is written (flicker-free
    fast path);
  - `Stop` before the delay elapses writes nothing and does not panic.
- `withSpinner`: `IsTTY=false` → `fn` runs, zero output; an error from `fn`
  propagates unchanged.
- Existing command tests (`runCLI` uses `IsTTY=false`) remain green with no
  changes — proving non-TTY behavior is untouched.

## Non-goals (YAGNI)

- No percentage/determinate progress, no multi-line output.
- No configurable delay (single constant).
- No spinner for the lifecycle commands (they already have one).
