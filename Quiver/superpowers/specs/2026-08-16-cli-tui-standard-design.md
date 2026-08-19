# CLI TUI Standard — Design

Date: 2026-08-16
Branch: `feature/cli`
Status: approved, pending implementation plan

## 1. Problem

Every `quiver` command renders its result its own way. Output shape, empty
states, spinner behaviour, glyphs, colours and exit codes were each decided at
the call site, so equivalent situations produce inequivalent output: an empty
catalog and a filter that matched nothing are byte-identical, `-o yaml` is
silently ignored by some commands, and a missing argument exits 1 where every
other usage failure exits 2.

This design replaces per-command rendering with a small set of shared
interfaces, so that a command declares *what* it fetches and *what* it shows,
and the standard decides *how* it is drawn, serialized, and exited.

## 2. Decisions

Four decisions frame everything below. They were settled before the design and
are not revisited here.

1. **Every command is a `tea.Program`.** One uniform code path; the non-TTY
   case is handled by running the same program with `tea.WithoutRenderer()`.
2. **`CommandModel` exposes `View()` and `Payload()`.** `View()` is the TTY
   rendering; `Payload()` is the structured result. `json`/`yaml` serialize
   `Payload()` outside the model, so Bubble Tea never learns that formats
   exist.
3. **All I/O happens inside the model, via `tea.Cmd`.** `loading` is a real
   state in every command. The spinner stops because the result message
   arrived, never because a timer said so.
4. **Taxonomy is lifecycle × view component.** Three flows govern when
   messages arrive; four components govern what is drawn. A command picks one
   flow and composes components.

**No new dependencies.** `bubbletea v1.3.10` and `lipgloss v1.1.0` are already
in `go.mod`; nothing else is added. In particular `bubbles` is not used — the
spinner is hand-rolled in `theme/`.

The following APIs were verified present at the pinned versions:
`tea.WithoutRenderer`, `tea.WithInput`, `tea.WithContext`, `tea.WithOutput`,
`tea.Tick`, `tea.Batch`, `lipgloss.NewRenderer`, `(*Renderer).NewStyle`,
`(*Renderer).SetColorProfile`.

## 3. Package layout

```
internal/cli/tui/
  tui.go          CommandModel, Runner, Format
  errors.go       usageError, connError, exit-code mapping
  flow/           Instant, Transactional, Streaming, Confirm
    msg.go        shared message set
  component/      Table, Fields, Steps, Outcome
  theme/          Theme, Spinner, state -> (glyph, colour)
```

The message set lives in `flow/` rather than beside `tui.go`: only the flows
construct or consume it, and `tui` has no reason to depend on it.

The sub-package is named `flow`, not `lifecycle`: `lifecycle` already denotes
arrow lifecycle methods (`_install`, `_execute`, …) in this codebase and there
is an existing `internal/cli/lifecycle` package. Reusing the word would be
misleading.

## 4. The contract

```go
// CommandModel is the contract every command's model satisfies.
type CommandModel interface {
	tea.Model
	Payload() any // structured result, serialized for json and yaml
	Err() error   // terminal outcome; nil on success
}
```

`Runner` is the only place that knows about TTYs and formats:

```go
type Format int

const (
	FormatTable Format = iota
	FormatJSON
	FormatYAML
)

type Runner struct {
	format Format
	tty    bool
	out    io.Writer
	lip    *lipgloss.Renderer
}

func NewRunner(out io.Writer, format Format, tty bool) Runner
func (r Runner) Run(ctx context.Context, m CommandModel) error
```

`Run` behaves as follows:

| Condition | Program options | After the program exits |
|---|---|---|
| TTY, `FormatTable` | `WithContext` | nothing; tea already drew it |
| non-TTY, `FormatTable` | `+ WithoutRenderer, WithInput(nil)` | write `cm.View()` |
| any TTY, `FormatJSON` | `+ WithoutRenderer, WithInput(nil)` | encode `cm.Payload()` |
| any TTY, `FormatYAML` | `+ WithoutRenderer, WithInput(nil)` | encode `cm.Payload()` |

If `cm.Err() != nil`, `Run` returns that error and writes no result. The
default format is `FormatTable` on a TTY and `FormatJSON` when stdout is piped,
preserving today's behaviour.

The `lipgloss.Renderer` is bound to `out` rather than using the package-global
renderer. This makes colour-profile detection correct for the actual
destination and lets tests render into a buffer with a forced profile.

## 5. Flows

Each flow is a generic struct that *is* a `CommandModel`. Commands configure
it with closures rather than embedding and re-wrapping it, which keeps
per-command code declarative.

```go
// Instant: fetch, render, quit.
func NewInstant[T any](
	th theme.Theme,
	label string,
	fetch func() (T, error),
	view func(T, theme.Theme) string,
) *Instant[T]

// Transactional: mutate, report outcome. A generic type alias for Instant,
// because their mechanics are identical: one round trip, one terminal result.
// The separate constructor names the intent at call sites and lets the two
// diverge later without touching them.
type Transactional[T any] = Instant[T]

type TxOpts[T any] struct {
	Label string
	Do    func() (T, error)
	View  func(T, theme.Theme) string
}

func NewTransactional[T any](th theme.Theme, o TxOpts[T]) *Transactional[T]

// Streaming: subscribe, render evolving state, report.
type StreamOpts[T any] struct {
	Label string
	Start func() (<-chan Event[T], error)
	View  func(steps []component.Step, final *T, t theme.Theme) string
}

func NewStreaming[T any](th theme.Theme, o StreamOpts[T]) *Streaming[T]
```

None of the work closures take a `context.Context`. `tea.Cmd` is
`func() tea.Msg` and accepts no arguments, and CLAUDE.md forbids storing a
context in a struct, so the command's closure captures `cmd.Context()` at
construction and the flow never holds one. The same context is passed to
`tea.WithContext`, so both sides unwind together on cancellation.

`Event[T]` is the flow-level union of what the runtime WebSocket reports: a
step beginning, a step completing or failing, and the terminal result, which
carries the final payload as `Final *T`. The `client` package translates its
wire DTOs into `Event[T]`; `flow` does not import transport types.

`View`'s `final` parameter is `nil` for every frame until the run reaches its
terminal result, which is how the view knows to render `Steps` rather than
`Outcome`.

### 5.1 Confirm is a decorator, not a flow

`uninstall` both prompts for confirmation and streams steps, so confirmation
cannot be a property of any single flow. It is a wrapper:

```go
func Confirm(prompt string, next CommandModel) CommandModel
```

`Confirm` renders the prompt, waits for `y`/`n`, then delegates entirely to
`next`, forwarding `Payload()` and `Err()`. Aborting yields `ExitOK` with no
mutation.

Rules:

- When `--yes` is set, `Confirm` is not applied at all; the command constructs
  `next` directly.
- On a non-TTY without `--yes`, `Confirm` returns a `usageError` (exit 2).
  Prompting is impossible and silently proceeding would be unsafe.

### 5.2 How the theme reaches a view

View closures receive `theme.Theme` as a parameter rather than reading a
package global. The `Theme` is derived from the `lipgloss.Renderer` that
`Runner` bound to its output writer, so it carries the correct colour profile
for the actual destination.

The `Theme` is passed to the flow constructor, not injected after
construction. `Runner` exposes `Theme() theme.Theme`, and the cobra `RunE`
that already holds the `Runner` threads it into the model constructor:

```go
m := newListModel(runner.Theme(), c)
return runner.Run(cmd.Context(), m)
```

Injection after construction was considered and rejected: it would require an
unexported setter on `CommandModel`, and a type in package `flow` cannot
implement an unexported method declared in package `tui`. Exporting the setter
would put a mutator on the public interface for no benefit. Constructor
injection keeps `CommandModel` to the three members in §4.

This is what makes golden-file tests colour-stable: a test constructs a
`Runner` over a buffer with the ASCII profile forced, and every component
below it renders without escape sequences.

### 5.3 Example command

```go
func newListModel(c *client.Client) tui.CommandModel {
	return flow.NewInstant("loading catalog",
		func(ctx context.Context) (catalog, error) { return fetchCatalog(ctx, c) },
		func(d catalog, t theme.Theme) string {
			return component.Table(arrowCols, d.arrowRows(), t) +
				component.Table(collCols, d.collectionRows(), t)
		})
}
```

## 6. Message set

One vocabulary across all flows, defined in `msg.go`:

| Message | Emitted by | Effect |
|---|---|---|
| `theme.TickMsg` | spinner tick chain | advance frame while `!settled` |
| `readyMsg[T]` | `fetch` / `Do` cmd | store data, `settled = true`, `tea.Quit` |
| `openedMsg[T]` | `Start` cmd | store the channel, request the first event |
| `eventMsg[T]` | streaming event pump | append or update a step, re-render |
| `errMsg` | any cmd | store error, `tea.Quit` |

`Confirm` needs no message of its own — it reads `tea.KeyMsg` directly and
switches on `key.String()`.

`Streaming` swaps the component it renders as state advances: spinner →
`Steps` on the first `stepMsg` → `Outcome` on `readyMsg`. This transition needs
no special case, and is the main thing the lifecycle × component split buys.

Cancellation flows through `tea.WithContext(ctx)`, so Ctrl-C and a cancelled
parent context unwind identically; the in-flight request is bound to the same
`ctx`.

## 7. Components

Four pure functions. No `tea.Model`, no state:

```go
func Table(cols []Column, rows [][]string, t theme.Theme) string
func Fields(title string, pairs []Field, t theme.Theme) string
func Steps(steps []Step, t theme.Theme) string
func Outcome(r Result, t theme.Theme) string
```

Purity is what makes a "streaming table" a non-problem: `Streaming.View` may
call `component.Table` directly.

Two contracts are mandatory, because they are precisely the inconsistencies
this standard exists to remove:

- **`Table` never renders bare headers over nothing.** An empty row set
  renders an explicit empty state, parameterised so that "no arrows yet" and
  "no arrows match `-F ffmpeg`" are distinguishable.
- **`Fields` distinguishes absent from empty.** Absent fields are omitted;
  fields that are present but empty render `—`. Silently dropping empty rows
  makes two entities render different row counts with no signal.

`Fields` takes a `title` so grouped panels are expressed as repeated `Fields`
blocks rather than requiring a fifth component.

`theme/` owns a single state → (glyph, colour) table for the whole CLI, so
`● ready`, `▶ running`, `○ absent` are defined once.

### 7.1 Spinner

Hand-rolled in `theme/`, no `bubbles`:

```go
type Spinner struct {
	frames []string
	idx    int
	start  time.Time
	delay  time.Duration
}

type TickMsg time.Time

func (s Spinner) Tick() tea.Cmd
func (s Spinner) Update(tea.Msg) (Spinner, tea.Cmd)
func (s Spinner) View() string // "" until delay has elapsed
```

Frames: `⣾⣽⣻⢿⡿⣟⣯⣷`. `delay` defaults to 120ms and preserves the existing
don't-flash-on-fast-commands behaviour.

The tick chain is armed exactly once, in `Init()`, and is not re-armed once
`settled`. Arming it in both `Init()` and `Update()` would double the chain and
the frame rate.

## 8. Errors and exit codes

Exit code is a function of error type, computed in exactly one place:

```go
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnreachable = 3
)

func CodeFor(err error) int // usageError -> 2, connError -> 3, nil -> 0, else 1
```

`Err()` → `Runner.Run` returns it → `main` prints `quiver: <err>` to stderr and
exits `CodeFor(err)`.

Two rules:

- **Cobra's own argument errors are wrapped as `usageError`.** Custom `Args`
  validators return the typed error directly, so `quiver search` with no
  pattern exits 2 like every other usage failure. It exits 1 today.
- **A failed `Streaming` run keeps its frame.** Steps rendered so far remain on
  screen with the failing step marked, then the error line is printed. The
  trace of where execution died must not be erased.

## 9. Command assignment

The full command surface, with its flow and components. `Confirm ∘ X` denotes
the decorator applied to flow `X`.

| Command | Flow | Components |
|---|---|---|
| `health` | Instant | Outcome |
| `version` | Instant | Fields |
| `version --client-only` | Instant (local) | Fields |
| `list [-F]` | Instant | Table × 2 |
| `search <pattern>` | Instant | Table × 2 |
| `info <ns>` | Instant | Fields |
| `info <ns> --manifest` | Instant | none; `Payload()` only |
| `methods <ns>` | Instant | Table |
| `<ns>` (bare panel) | Instant (local) | Fields × 3 |
| `ps [--all]` | Instant | Table |
| `status [ns]` | Instant | Table |
| `install <ns>` | Streaming | Steps → Outcome |
| `run <ns>` | Streaming | Steps → Outcome |
| `stop <ns>` | Streaming | Steps → Outcome |
| `update <ns>` | Streaming | Steps → Outcome |
| `uninstall <ns>` | Confirm ∘ Streaming | Steps → Outcome |
| `<ns> <method>` | Streaming | Steps → Outcome |
| `arrow add <ns>` | Transactional | Outcome |
| `arrow remove <ns>` | Confirm ∘ Transactional | Outcome |
| `arrow refresh <ns>` | Transactional | Outcome |
| `arrow list` | Instant | Table |
| `arrow show <ns>` | Instant | Fields |
| `collection follow <ns>` | Transactional | Outcome |
| `collection unfollow <ns>` | Confirm ∘ Transactional | Outcome |
| `collection update <ns>` | Transactional | Outcome |
| `collection list` | Instant | Table |
| `collection show <ns>` | Instant | Fields |
| `context add` | Transactional (local) | Outcome |
| `context use` | Transactional (local) | Outcome |
| `context remove` | Confirm ∘ Transactional (local) | Outcome |
| `context current` | Instant (local) | Fields |
| `context list` | Instant (local) | Table |
| `context show` | Instant (local) | Fields |

"local" marks commands that touch no daemon. They still run through a flow and
still perform their work inside a `tea.Cmd`; the work is a file read or write
rather than a request. This keeps one lifecycle rather than two.

Two consequences worth noting, both of which fall out of the standard rather
than being special-cased:

- `info --manifest` gains working `-o yaml`. It sets `Payload()` to the
  manifest and renders nothing, so format handling is the `Runner`'s as usual.
  Today it bypasses the renderer and ignores `-o`.
- Every command honours `--output`. The current split between commands that
  respect it and commands that always print plain text disappears, because
  format is resolved in one place.

  This is an accepted user-visible behaviour change, approved 2026-08-16.
  Commands that today always print plain text — `install`, `run`, `stop`,
  `update`, `uninstall`, `arrow add`, `arrow remove`, `arrow refresh`,
  `collection follow`, `collection unfollow`, `collection update`, the
  `context` mutations, `health` and `version` — will emit JSON when stdout is
  piped, matching the rest of the CLI. Anything scripting against their
  current plain-text output must pass `-o table` to keep it.

## 10. Testing

Coverage target is the project standard: ≥ 95% per new implementation.

| Layer | Style | Cardinality |
|---|---|---|
| `component/` | golden strings, forced ASCII profile | once per component |
| `theme/` | glyph/colour table, spinner delay and frame advance | once |
| `flow/` | drive `Update()` with the message set, fake fetch | once per flow |
| `Confirm` | y / n / non-TTY-without-`--yes` | once |
| `Runner` | table over (tty × format) → assert stdout bytes | once |
| commands | wiring only: correct flow, label, `Payload()` shape | thin, per command |

The generic flows are what make the coverage gate affordable: loading, error
and cancellation paths are proven once in `flow/`, so the per-command burden
collapses to asserting wiring. Golden files render through a `lipgloss.Renderer`
bound to a buffer with the ASCII profile forced, so tests are colour-stable
regardless of the terminal running them.

## 11. Out of scope

- `quiver daemon` — it is the server process running in the foreground, not a
  command that renders a result. It is unaffected.
- Interactive list commands (scroll, filter, select). Every flow here quits as
  soon as it has a terminal result. Interactivity would be a later addition
  built on the same `CommandModel`.
- Behaviour changes beyond the rendering contracts stated in §7 and §8. In
  particular, making the bare-namespace panel fetch and name an arrow's real
  custom methods is now *possible* (it has a flow that can fetch) but is not
  part of this work.

## 12. Migration

The standard is additive: `internal/cli/tui/` is new and does not modify
existing packages. Commands move onto it one at a time, each move deleting its
bespoke rendering. The work is orderable by flow — all `Instant` commands, then
`Transactional`, then `Streaming` — so each stage exercises one flow against
many commands before the next flow is written.
