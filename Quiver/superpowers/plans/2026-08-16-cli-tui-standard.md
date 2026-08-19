# CLI TUI Standard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `internal/cli/tui/` — the shared rendering framework every Quiver CLI command will be migrated onto, so that output shape, empty states, spinner behaviour, glyphs and exit codes are decided once instead of per command.

**Architecture:** One `CommandModel` contract (`tea.Model` + `Payload()` + `Err()`). Three flows govern when messages arrive (`Instant`, `Transactional`, `Streaming`) with `Confirm` as a decorator; four pure components govern what is drawn (`Table`, `Fields`, `Steps`, `Outcome`). A single `Runner` owns TTY detection and format serialization, so Bubble Tea never learns that `--output` exists.

**Tech Stack:** Go 1.26.2, `bubbletea v1.3.10`, `lipgloss v1.1.0`, `gopkg.in/yaml.v3`, `testify v1.11.x`. All already present in `go.mod`.

**Spec:** `docs/superpowers/specs/2026-08-16-cli-tui-standard-design.md`

**Scope:** This plan builds the framework only. Migrating existing commands onto it is a follow-up plan, written after the framework exists.

## Global Constraints

- **No new dependencies.** Nothing may be added to `go.mod`. In particular `bubbles` is not used (the spinner is hand-rolled) and `termenv` must not be imported directly — it is an indirect dependency and importing it would promote it to direct.
- Go 1.26.2.
- Run `make fmt` (gofumpt + goimports, local prefix `github.com/rabbytesoftware/quiver`) before every commit. Never hand-order imports.
- No `init()` functions. No mutable package-level variables — only typed constants, `errors.New` sentinels, and compile-time interface checks (`var _ I = (*T)(nil)`).
- `funlen` ≤ 100 lines / 50 statements. `gocyclo` ≤ 15. `nestif` ≤ 2. `errcheck`: every error checked.
- `exhaustive`: a switch on a named type must cover every value, and a `default` case does **not** satisfy it. Cover all cases explicitly with no `default`.
- Doc comments on exported symbols: one sentence starting with the symbol name, ending with a period. English only.
- Error messages: lowercase first letter, no trailing period, colon-separated context chain, wrapped with `%w`.
- Tests: `TestType_Method_Description` naming, table-driven with a `testCases` slice, `require` for fatal assertions and `assert` for non-fatal. Coverage ≥ 95% for every file in this plan.
- Tests must be colour-stable: construct `lipgloss.NewRenderer(&buf)` over a `bytes.Buffer` (auto-detects the ASCII profile), and set `t.Setenv("NO_COLOR", "1")` plus `t.Setenv("CLICOLOR_FORCE", "0")` so a CI terminal cannot force colour on.

## Design refinements made during planning

Three points where the plan is more precise than the spec. All three are already reflected in the spec text.

1. **Theme is passed to flow constructors**, not injected afterwards. A type in package `flow` cannot implement an unexported method declared in package `tui`, and exporting a setter would put a mutator on the public interface.
2. **Fetch closures take no `context.Context`.** `tea.Cmd` is `func() tea.Msg` and accepts no arguments, and CLAUDE.md forbids storing a context in a struct. The command's closure captures `cmd.Context()` at construction; the flow never holds one. The same context is passed to `tea.WithContext`, so cancellation unwinds both sides together.
3. **`Transactional` is a generic type alias for `Instant`.** Their mechanics are identical — one round trip, one terminal result. A distinct constructor preserves intent at call sites and lets the two diverge later without touching them, but duplicating the engine would be dead weight.

## File Structure

| File | Responsibility |
|---|---|
| `internal/cli/tui/theme/theme.go` | `Theme`, styles bound to a renderer, arrow-state glyph table |
| `internal/cli/tui/theme/spinner.go` | `Spinner`, `TickMsg`; tick-counted, no clock |
| `internal/cli/tui/component/table.go` | `Table`, `Column`; column widths, mandatory empty state |
| `internal/cli/tui/component/fields.go` | `Fields`, `Field`; absent vs present-but-empty |
| `internal/cli/tui/component/steps.go` | `Steps`, `Step`, `StepState` |
| `internal/cli/tui/component/outcome.go` | `Outcome`, `Result` |
| `internal/cli/tui/errors.go` | `usageError`, `connError`, `CodeFor`, exit constants |
| `internal/cli/tui/tui.go` | `CommandModel`, `Format`, `ParseFormat`, `Runner` |
| `internal/cli/tui/flow/msg.go` | `readyMsg`, `errMsg`, `openedMsg`, `eventMsg` |
| `internal/cli/tui/flow/instant.go` | `Instant[T]` — the shared engine |
| `internal/cli/tui/flow/transactional.go` | `Transactional[T]` alias + `NewTransactional` |
| `internal/cli/tui/flow/streaming.go` | `Streaming[T]`, `Event[T]`, `EventKind` |
| `internal/cli/tui/flow/confirm.go` | `Confirm` decorator, `ConfirmGuard` |

---

### Task 1: Theme and the state glyph table

**Files:**
- Create: `internal/cli/tui/theme/theme.go`
- Test: `internal/cli/tui/theme/theme_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `theme.Theme` struct with exported style fields `Header`, `Label`, `Value`, `Muted`, `OK`, `Warn`, `Fail`, `Active` (all `lipgloss.Style`); `theme.New(r *lipgloss.Renderer) Theme`; `func (t Theme) State(s domain.ArrowState) string`.

- [ ] **Step 1: Write the failing test**

```go
package theme_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer
	return theme.New(lipgloss.NewRenderer(&buf))
}

func TestTheme_State_KnownStates(t *testing.T) {
	th := newTestTheme(t)

	testCases := []struct {
		name  string
		state domain.ArrowState
		want  string
	}{
		{"ready", domain.ArrowStateReady, "● ready"},
		{"running", domain.ArrowStateRunning, "▶ running"},
		{"absent", domain.ArrowStateAbsent, "○ absent"},
		{"installing", domain.ArrowStateInstalling, "⇣ installing"},
		{"outdated", domain.ArrowStateOutdated, "▲ outdated"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, th.State(tc.state))
		})
	}
}

func TestTheme_State_UnknownStateFallsBack(t *testing.T) {
	th := newTestTheme(t)
	assert.Equal(t, "? nonsense", th.State(domain.ArrowState("nonsense")))
}

func TestTheme_New_StylesAreUsable(t *testing.T) {
	th := newTestTheme(t)
	require.Equal(t, "hi", th.Muted.Render("hi"))
	require.Equal(t, "hi", th.Header.Render("hi"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/theme/ -run TestTheme -v`
Expected: FAIL — package `theme` does not exist.

- [ ] **Step 3: Write minimal implementation**

```go
// Package theme holds the CLI's visual vocabulary: colours, glyphs and the
// spinner, all bound to a single lipgloss renderer.
package theme

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Theme carries the styles and glyphs every component draws with.
type Theme struct {
	Header lipgloss.Style
	Label  lipgloss.Style
	Value  lipgloss.Style
	Muted  lipgloss.Style
	OK     lipgloss.Style
	Warn   lipgloss.Style
	Fail   lipgloss.Style
	Active lipgloss.Style

	states map[domain.ArrowState]stateGlyph
}

type stateGlyph struct {
	glyph string
	style func(Theme) lipgloss.Style
}

// New returns a Theme whose styles are bound to r.
func New(r *lipgloss.Renderer) Theme {
	return Theme{
		Header: r.NewStyle().Bold(true),
		Label:  r.NewStyle().Bold(true),
		Value:  r.NewStyle(),
		Muted:  r.NewStyle().Foreground(lipgloss.Color("8")),
		OK:     r.NewStyle().Foreground(lipgloss.Color("2")),
		Warn:   r.NewStyle().Foreground(lipgloss.Color("3")),
		Fail:   r.NewStyle().Foreground(lipgloss.Color("1")),
		Active: r.NewStyle().Foreground(lipgloss.Color("6")),
		states: stateTable(),
	}
}

func stateTable() map[domain.ArrowState]stateGlyph {
	ok := func(t Theme) lipgloss.Style { return t.OK }
	muted := func(t Theme) lipgloss.Style { return t.Muted }
	active := func(t Theme) lipgloss.Style { return t.Active }
	warn := func(t Theme) lipgloss.Style { return t.Warn }

	return map[domain.ArrowState]stateGlyph{
		domain.ArrowStateAbsent:       {"○", muted},
		domain.ArrowStateReady:        {"●", ok},
		domain.ArrowStateRunning:      {"▶", active},
		domain.ArrowStateStopping:     {"◼", active},
		domain.ArrowStateDraining:     {"◍", active},
		domain.ArrowStateDetached:     {"◈", active},
		domain.ArrowStateInstalling:   {"⇣", active},
		domain.ArrowStateUninstalling: {"⇡", active},
		domain.ArrowStateUpdating:     {"⇅", active},
		domain.ArrowStateOutdated:     {"▲", warn},
		domain.ArrowStateRemoved:      {"✕", muted},
	}
}

// State renders s as a coloured glyph followed by its name.
func (t Theme) State(s domain.ArrowState) string {
	g, ok := t.states[s]
	if !ok {
		return t.Muted.Render("? " + string(s))
	}
	return g.style(t).Render(g.glyph + " " + string(s))
}
```

Note: `stateTable()` is a function, not a package-level map, because mutable package-level variables are banned. All eleven `domain.ArrowState*` constant names above are verified against `internal/domain/arrow.go:67-79` and are exact.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/theme/ -run TestTheme -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/theme/theme.go internal/cli/tui/theme/theme_test.go
git commit -m "feat(cli/tui): theme with bound styles and arrow-state glyphs"
```

---

### Task 2: Spinner

**Files:**
- Create: `internal/cli/tui/theme/spinner.go`
- Test: `internal/cli/tui/theme/spinner_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `theme.TickMsg` (`time.Time`); `theme.NewSpinner() Spinner`; `func (s Spinner) Tick() tea.Cmd`; `func (s Spinner) Update(tea.Msg) (Spinner, tea.Cmd)`; `func (s Spinner) Frame() string`.

The spinner counts ticks rather than reading a clock, so its start delay is deterministic and needs no injected time source.

- [ ] **Step 1: Write the failing test**

```go
package theme_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func TestSpinner_Frame_HiddenUntilDelayElapsed(t *testing.T) {
	s := theme.NewSpinner()
	assert.Equal(t, "", s.Frame(), "spinner must not flash on fast commands")

	s, _ = s.Update(theme.TickMsg(time.Now()))
	assert.Equal(t, "", s.Frame())

	s, _ = s.Update(theme.TickMsg(time.Now()))
	assert.NotEqual(t, "", s.Frame(), "spinner must appear after the start delay")
}

func TestSpinner_Update_AdvancesFrameAndRearms(t *testing.T) {
	s := theme.NewSpinner()
	for range 2 {
		s, _ = s.Update(theme.TickMsg(time.Now()))
	}

	first := s.Frame()
	s, cmd := s.Update(theme.TickMsg(time.Now()))

	assert.NotEqual(t, first, s.Frame(), "frame must advance")
	require.NotNil(t, cmd, "tick must re-arm itself")
}

func TestSpinner_Update_IgnoresOtherMessages(t *testing.T) {
	s := theme.NewSpinner()
	for range 2 {
		s, _ = s.Update(theme.TickMsg(time.Now()))
	}

	before := s.Frame()
	s, cmd := s.Update("not a tick")

	assert.Equal(t, before, s.Frame())
	assert.Nil(t, cmd)
}

func TestSpinner_Tick_ReturnsTickMsg(t *testing.T) {
	msg := theme.NewSpinner().Tick()()
	assert.IsType(t, theme.TickMsg{}, msg)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/theme/ -run TestSpinner -v`
Expected: FAIL — `undefined: theme.NewSpinner`.

- [ ] **Step 3: Write minimal implementation**

```go
package theme

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg advances the spinner one frame.
type TickMsg time.Time

const (
	spinnerFPS      = 80 * time.Millisecond
	spinnerMinTicks = 2 // ~160ms; suppresses the flash on fast commands
)

// Spinner is an indeterminate progress indicator driven by tick messages.
type Spinner struct {
	frames []string
	idx    int
	ticks  int
}

// NewSpinner returns a Spinner that stays hidden until its start delay elapses.
func NewSpinner() Spinner {
	return Spinner{frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}}
}

// Tick returns the command that schedules the next frame.
func (s Spinner) Tick() tea.Cmd {
	return tea.Tick(spinnerFPS, func(t time.Time) tea.Msg { return TickMsg(t) })
}

// Update advances the spinner and re-arms the tick chain.
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	if _, ok := msg.(TickMsg); !ok {
		return s, nil
	}
	s.ticks++
	s.idx = (s.idx + 1) % len(s.frames)
	return s, s.Tick()
}

// Frame returns the current frame, or an empty string before the start delay.
func (s Spinner) Frame() string {
	if s.ticks < spinnerMinTicks {
		return ""
	}
	return s.frames[s.idx]
}
```

The tick chain must be armed exactly once, in a model's `Init()`. Arming it in both `Init()` and `Update()` doubles the chain and the frame rate.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/theme/ -run TestSpinner -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/theme/spinner.go internal/cli/tui/theme/spinner_test.go
git commit -m "feat(cli/tui): tick-counted spinner with start delay"
```

---

### Task 3: Table component

**Files:**
- Create: `internal/cli/tui/component/table.go`
- Test: `internal/cli/tui/component/table_test.go`

**Interfaces:**
- Consumes: `theme.Theme`.
- Produces: `component.Column{Title string}`; `func Table(cols []Column, rows [][]string, empty string, t theme.Theme) string`.

`Table` must never render bare headers over nothing — an empty row set renders the caller-supplied `empty` message instead, which is what makes "no arrows yet" distinguishable from "nothing matched `-F ffmpeg`".

- [ ] **Step 1: Write the failing test**

```go
package component_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer
	return theme.New(lipgloss.NewRenderer(&buf))
}

func TestTable_Render_PadsColumnsToWidestCell(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "NAME"}, {Title: "STATE"}}
	rows := [][]string{{"repo", "ready"}, {"much-longer-name", "running"}}

	got := component.Table(cols, rows, "none", th)

	assert.Equal(t, ""+
		"NAME              STATE\n"+
		"repo              ready\n"+
		"much-longer-name  running\n", got)
}

func TestTable_Render_EmptyRowsRendersEmptyMessageNotHeaders(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "NAME"}, {Title: "STATE"}}

	got := component.Table(cols, nil, "no arrows match -F ffmpeg", th)

	assert.Equal(t, "no arrows match -F ffmpeg\n", got)
	assert.NotContains(t, got, "NAME", "must not print headers over nothing")
}

func TestTable_Render_ShortRowsArePadded(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "A"}, {Title: "B"}}

	got := component.Table(cols, [][]string{{"only"}}, "none", th)

	assert.Equal(t, "A     B\nonly  \n", got)
}

func TestTable_Render_ExtraCellsAreTruncated(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "A"}}

	got := component.Table(cols, [][]string{{"x", "dropped"}}, "none", th)

	assert.Equal(t, "A\nx\n", got)
	assert.NotContains(t, got, "dropped")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/component/ -run TestTable -v`
Expected: FAIL — `undefined: component.Table`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package component holds the CLI's pure view components. Every function here
// is a total function of its data and theme, with no state and no I/O.
package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

const colGap = 2

// Column is a table heading.
type Column struct {
	Title string
}

// Table renders rows beneath cols. When rows is empty it renders empty instead
// of a header row, so that an empty result and a filtered-out result differ.
func Table(cols []Column, rows [][]string, empty string, t theme.Theme) string {
	if len(cols) == 0 || len(rows) == 0 {
		return t.Muted.Render(empty) + "\n"
	}

	widths := columnWidths(cols, rows)

	var b strings.Builder
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.Title
	}
	b.WriteString(t.Header.Render(joinCells(titles, widths)) + "\n")

	for _, row := range rows {
		b.WriteString(joinCells(row, widths) + "\n")
	}
	return b.String()
}

func columnWidths(cols []Column, rows [][]string) []int {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = lipgloss.Width(c.Title)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// joinCells pads cells to widths, dropping extras and padding short rows. The
// final column is not padded, so lines carry no trailing run of spaces.
func joinCells(cells []string, widths []int) string {
	var b strings.Builder
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		if i == len(widths)-1 {
			b.WriteString(cell)
			break
		}
		b.WriteString(cell)
		b.WriteString(strings.Repeat(" ", w-lipgloss.Width(cell)+colGap))
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/component/ -run TestTable -v`
Expected: PASS. If the padding assertions are off by the trailing-cell rule, fix the *test's* expected strings to match the documented behaviour (last column unpadded) rather than adding trailing spaces to the implementation.

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/component/table.go internal/cli/tui/component/table_test.go
git commit -m "feat(cli/tui): table component with mandatory empty state"
```

---

### Task 4: Fields component

**Files:**
- Create: `internal/cli/tui/component/fields.go`
- Test: `internal/cli/tui/component/fields_test.go`

**Interfaces:**
- Consumes: `theme.Theme`.
- Produces: `component.Field{Label string; Value string; Set bool}`; `func Fields(title string, fields []Field, t theme.Theme) string`.

Absent fields (`Set: false`) are omitted. Fields that are set but empty render an em dash, so two entities never differ in row count with no signal as to why.

- [ ] **Step 1: Write the failing test**

```go
func TestFields_Render_AlignsLabels(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{
		{Label: "Name", Value: "repo", Set: true},
		{Label: "Description", Value: "a thing", Set: true},
	}

	got := component.Fields("", fields, th)

	assert.Equal(t, ""+
		"Name         repo\n"+
		"Description  a thing\n", got)
}

func TestFields_Render_OmitsAbsentButMarksEmpty(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{
		{Label: "Name", Value: "repo", Set: true},
		{Label: "License", Value: "", Set: true},
		{Label: "Tags", Value: "", Set: false},
	}

	got := component.Fields("", fields, th)

	assert.Contains(t, got, "License")
	assert.Contains(t, got, "—", "set-but-empty must be visible")
	assert.NotContains(t, got, "Tags", "absent fields are omitted")
}

func TestFields_Render_TitleIsPrefixed(t *testing.T) {
	th := newTestTheme(t)
	fields := []component.Field{{Label: "A", Value: "b", Set: true}}

	got := component.Fields("Lifecycle", fields, th)

	assert.Equal(t, "Lifecycle\nA  b\n", got)
}

func TestFields_Render_AllAbsentRendersNothing(t *testing.T) {
	th := newTestTheme(t)
	got := component.Fields("", []component.Field{{Label: "A", Set: false}}, th)
	assert.Equal(t, "", got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/component/ -run TestFields -v`
Expected: FAIL — `undefined: component.Fields`.

- [ ] **Step 3: Write minimal implementation**

```go
package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

const emptyValue = "—"

// Field is one label/value pair. Set distinguishes a field that is absent from
// one that is present but empty.
type Field struct {
	Label string
	Value string
	Set   bool
}

// Fields renders label/value pairs beneath an optional title. Fields that are
// not set are omitted; fields that are set but empty render an em dash.
func Fields(title string, fields []Field, t theme.Theme) string {
	shown := make([]Field, 0, len(fields))
	width := 0
	for _, f := range fields {
		if !f.Set {
			continue
		}
		shown = append(shown, f)
		if w := lipgloss.Width(f.Label); w > width {
			width = w
		}
	}
	if len(shown) == 0 {
		return ""
	}

	var b strings.Builder
	if title != "" {
		b.WriteString(t.Header.Render(title) + "\n")
	}
	for _, f := range shown {
		value := f.Value
		if value == "" {
			value = t.Muted.Render(emptyValue)
		}
		pad := strings.Repeat(" ", width-lipgloss.Width(f.Label)+colGap)
		b.WriteString(t.Label.Render(f.Label) + pad + value + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/component/ -run TestFields -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/component/fields.go internal/cli/tui/component/fields_test.go
git commit -m "feat(cli/tui): fields component distinguishing absent from empty"
```

---

### Task 5: Steps and Outcome components

**Files:**
- Create: `internal/cli/tui/component/steps.go`, `internal/cli/tui/component/outcome.go`
- Test: `internal/cli/tui/component/steps_test.go`, `internal/cli/tui/component/outcome_test.go`

**Interfaces:**
- Consumes: `theme.Theme`.
- Produces: `component.StepState` (`StepPending`, `StepRunning`, `StepDone`, `StepFailed`); `component.Step{Name string; State StepState}`; `func Steps(steps []Step, t theme.Theme) string`; `component.Result{OK bool; Subject string; Message string}`; `func Outcome(r Result, t theme.Theme) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestSteps_Render_NumbersStepsAndMarksState(t *testing.T) {
	th := newTestTheme(t)
	steps := []component.Step{
		{Name: "fetch source", State: component.StepDone},
		{Name: "build", State: component.StepRunning},
		{Name: "link", State: component.StepPending},
	}

	got := component.Steps(steps, th)

	assert.Equal(t, ""+
		"✓ 1 of 3  fetch source\n"+
		"▸ 2 of 3  build\n"+
		"· 3 of 3  link\n", got)
}

func TestSteps_Render_FailedStepIsMarked(t *testing.T) {
	th := newTestTheme(t)
	got := component.Steps([]component.Step{
		{Name: "build", State: component.StepFailed},
	}, th)

	assert.Equal(t, "✗ 1 of 1  build\n", got)
}

func TestSteps_Render_EmptyIsEmpty(t *testing.T) {
	assert.Equal(t, "", component.Steps(nil, newTestTheme(t)))
}

func TestOutcome_Render_SuccessAndFailure(t *testing.T) {
	th := newTestTheme(t)

	testCases := []struct {
		name   string
		result component.Result
		want   string
	}{
		{
			name:   "success",
			result: component.Result{OK: true, Subject: "github.com/u/r", Message: "installed"},
			want:   "✓ github.com/u/r  installed\n",
		},
		{
			name:   "failure",
			result: component.Result{OK: false, Subject: "github.com/u/r", Message: "build failed"},
			want:   "✗ github.com/u/r  build failed\n",
		},
		{
			name:   "no message",
			result: component.Result{OK: true, Subject: "daemon"},
			want:   "✓ daemon\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, component.Outcome(tc.result, th))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/component/ -run 'TestSteps|TestOutcome' -v`
Expected: FAIL — `undefined: component.Steps`.

- [ ] **Step 3: Write minimal implementation**

`steps.go`:

```go
package component

import (
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// StepState is the execution state of a single lifecycle step.
type StepState int

const (
	// StepPending is a step that has not started.
	StepPending StepState = iota
	// StepRunning is the step currently executing.
	StepRunning
	// StepDone is a step that completed successfully.
	StepDone
	// StepFailed is a step that terminated in failure.
	StepFailed
)

// Step is one entry in a lifecycle run.
type Step struct {
	Name  string
	State StepState
}

// Steps renders an ordered, numbered step list.
func Steps(steps []Step, t theme.Theme) string {
	var b strings.Builder
	for i, s := range steps {
		b.WriteString(fmt.Sprintf("%s %d of %d  %s\n",
			stepGlyph(s.State, t), i+1, len(steps), s.Name))
	}
	return b.String()
}

func stepGlyph(s StepState, t theme.Theme) string {
	switch s {
	case StepPending:
		return t.Muted.Render("·")
	case StepRunning:
		return t.Active.Render("▸")
	case StepDone:
		return t.OK.Render("✓")
	case StepFailed:
		return t.Fail.Render("✗")
	}
	return t.Muted.Render("·")
}
```

The switch covers all four `StepState` values with no `default`, which is what the `exhaustive` linter requires. The trailing `return` satisfies the compiler and is unreachable for valid values.

`outcome.go`:

```go
package component

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Result is the terminal outcome of a command.
type Result struct {
	OK      bool
	Subject string
	Message string
}

// Outcome renders a single-line terminal result.
func Outcome(r Result, t theme.Theme) string {
	glyph := t.Fail.Render("✗")
	if r.OK {
		glyph = t.OK.Render("✓")
	}

	var b strings.Builder
	b.WriteString(glyph + " " + r.Subject)
	if r.Message != "" {
		b.WriteString(strings.Repeat(" ", colGap) + r.Message)
	}
	b.WriteString("\n")
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/component/ -run 'TestSteps|TestOutcome' -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/component/steps.go internal/cli/tui/component/outcome.go \
        internal/cli/tui/component/steps_test.go internal/cli/tui/component/outcome_test.go
git commit -m "feat(cli/tui): steps and outcome components"
```

---

### Task 6: Typed errors and exit codes

**Files:**
- Create: `internal/cli/tui/errors.go`
- Test: `internal/cli/tui/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `tui.ExitOK`, `tui.ExitFailure`, `tui.ExitUsage`, `tui.ExitUnreachable` (untyped int constants); `func tui.Usage(format string, a ...any) error`; `func tui.Conn(addr string, err error) error`; `func tui.CodeFor(err error) int`.

- [ ] **Step 1: Write the failing test**

```go
package tui_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func TestCodeFor_MapsErrorTypesToExitCodes(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, tui.ExitOK},
		{"plain error is failure", errors.New("boom"), tui.ExitFailure},
		{"usage error", tui.Usage("unknown output format %q", "xml"), tui.ExitUsage},
		{"connection error", tui.Conn("http://h:9500", errors.New("refused")), tui.ExitUnreachable},
		{"wrapped usage error", fmt.Errorf("outer: %w", tui.Usage("bad")), tui.ExitUsage},
		{"wrapped conn error", fmt.Errorf("outer: %w", tui.Conn("h", errors.New("x"))), tui.ExitUnreachable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tui.CodeFor(tc.err))
		})
	}
}

func TestUsage_Message(t *testing.T) {
	assert.Equal(t, `unknown output format "xml"`,
		tui.Usage("unknown output format %q", "xml").Error())
}

func TestConn_MessageAndUnwrap(t *testing.T) {
	inner := errors.New("connection refused")
	err := tui.Conn("http://host:9500", inner)

	assert.Equal(t, "cannot reach daemon at http://host:9500: connection refused", err.Error())
	assert.ErrorIs(t, err, inner)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/ -run 'TestCodeFor|TestUsage|TestConn' -v`
Expected: FAIL — `undefined: tui.Usage`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package tui holds the CLI's shared rendering contract: the CommandModel
// every command satisfies, and the Runner that draws and serializes it.
package tui

import (
	"errors"
	"fmt"
)

// Process exit codes.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnreachable = 3
)

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// Usage returns an error that exits with ExitUsage.
func Usage(format string, a ...any) error {
	return usageError{msg: fmt.Sprintf(format, a...)}
}

type connError struct {
	addr string
	err  error
}

func (e connError) Error() string {
	return fmt.Sprintf("cannot reach daemon at %s: %v", e.addr, e.err)
}

func (e connError) Unwrap() error { return e.err }

// Conn returns an error that exits with ExitUnreachable.
func Conn(addr string, err error) error {
	return connError{addr: addr, err: err}
}

// CodeFor maps err to the process exit code it should produce.
func CodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	var ue usageError
	if errors.As(err, &ue) {
		return ExitUsage
	}

	var ce connError
	if errors.As(err, &ce) {
		return ExitUnreachable
	}

	return ExitFailure
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/ -run 'TestCodeFor|TestUsage|TestConn' -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/errors.go internal/cli/tui/errors_test.go
git commit -m "feat(cli/tui): typed usage and connection errors with exit mapping"
```

---

### Task 7: CommandModel, Format and Runner

**Files:**
- Create: `internal/cli/tui/tui.go`
- Test: `internal/cli/tui/tui_test.go`

**Interfaces:**
- Consumes: `tui.Usage` (Task 6), `theme.New` (Task 1).
- Produces: `tui.CommandModel` interface; `tui.Format` with `FormatTable`, `FormatJSON`, `FormatYAML`; `func tui.ParseFormat(s string) (Format, error)`; `func tui.NewRunner(out io.Writer, format Format, tty bool) Runner`; `func (r Runner) Theme() theme.Theme`; `func (r Runner) Run(ctx context.Context, m CommandModel) error`.

- [ ] **Step 1: Write the failing test**

```go
package tui_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// fakeModel is a CommandModel that quits immediately with a fixed result.
type fakeModel struct {
	view    string
	payload map[string]string
	err     error
}

func (m *fakeModel) Init() tea.Cmd                       { return tea.Quit }
func (m *fakeModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, tea.Quit }
func (m *fakeModel) View() string                        { return m.view }
func (m *fakeModel) Payload() any                        { return m.payload }
func (m *fakeModel) Err() error                          { return m.err }

func newFake() *fakeModel {
	return &fakeModel{view: "NAME\nrepo\n", payload: map[string]string{"name": "repo"}}
}

func TestParseFormat_KnownAndUnknown(t *testing.T) {
	testCases := []struct {
		name    string
		in      string
		want    tui.Format
		wantErr bool
	}{
		{"table", "table", tui.FormatTable, false},
		{"json", "json", tui.FormatJSON, false},
		{"yaml", "yaml", tui.FormatYAML, false},
		{"unknown", "xml", tui.FormatTable, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tui.ParseFormat(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRunner_Run_WritesPerFormat(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	testCases := []struct {
		name   string
		format tui.Format
		tty    bool
		want   string
	}{
		{"piped table writes the view", tui.FormatTable, false, "NAME\nrepo\n"},
		{"json encodes the payload", tui.FormatJSON, false, "{\n  \"name\": \"repo\"\n}\n"},
		{"yaml encodes the payload", tui.FormatYAML, false, "name: repo\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := tui.NewRunner(&buf, tc.format, tc.tty)

			require.NoError(t, r.Run(context.Background(), newFake()))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

func TestRunner_Run_ModelErrorIsReturnedAndSuppressesPayload(t *testing.T) {
	var buf bytes.Buffer
	r := tui.NewRunner(&buf, tui.FormatJSON, false)

	m := newFake()
	m.err = errors.New("not found")

	err := r.Run(context.Background(), m)

	require.ErrorContains(t, err, "not found")
	assert.Empty(t, buf.String(), "no result may be written when the command failed")
}

func TestRunner_Run_FailedTableRunKeepsItsFrame(t *testing.T) {
	var buf bytes.Buffer
	r := tui.NewRunner(&buf, tui.FormatTable, false)

	m := newFake()
	m.view = "✗ 2 of 7  build\n"
	m.err = errors.New("build failed")

	require.Error(t, r.Run(context.Background(), m))
	assert.Equal(t, "✗ 2 of 7  build\n", buf.String(), "the trace of where it died must survive")
}

func TestRunner_Theme_IsUsable(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	assert.Equal(t, "x", tui.NewRunner(&buf, tui.FormatTable, false).Theme().Muted.Render("x"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/ -run 'TestParseFormat|TestRunner' -v`
Expected: FAIL — `undefined: tui.ParseFormat`.

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// CommandModel is the contract every command's model satisfies.
type CommandModel interface {
	tea.Model
	// Payload returns the structured result, serialized for json and yaml.
	Payload() any
	// Err returns the terminal outcome, or nil on success.
	Err() error
}

// Format is an output encoding selected by --output.
type Format int

const (
	// FormatTable is the human-readable rendering.
	FormatTable Format = iota
	// FormatJSON is indented JSON.
	FormatJSON
	// FormatYAML is YAML.
	FormatYAML
)

// ParseFormat maps an --output flag value to a Format.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "table":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "yaml":
		return FormatYAML, nil
	}
	return FormatTable, Usage("unknown output format %q (table|json|yaml)", s)
}

// Runner draws a CommandModel and serializes its payload. It is the only place
// that knows whether stdout is a terminal or which format was requested.
type Runner struct {
	format Format
	tty    bool
	out    io.Writer
	theme  theme.Theme
}

// NewRunner returns a Runner writing to out. The lipgloss renderer is bound to
// out so colour-profile detection matches the real destination.
func NewRunner(out io.Writer, format Format, tty bool) Runner {
	return Runner{
		format: format,
		tty:    tty,
		out:    out,
		theme:  theme.New(lipgloss.NewRenderer(out)),
	}
}

// Theme returns the theme command models must render with.
func (r Runner) Theme() theme.Theme { return r.theme }

// Run executes m and writes its result in the configured format.
func (r Runner) Run(ctx context.Context, m CommandModel) error {
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithOutput(r.out)}
	if !r.tty || r.format != FormatTable {
		opts = append(opts, tea.WithoutRenderer(), tea.WithInput(nil))
	}

	final, err := tea.NewProgram(m, opts...).Run()
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	cm, ok := final.(CommandModel)
	if !ok {
		return fmt.Errorf("render: model %T is not a CommandModel", final)
	}

	if cerr := cm.Err(); cerr != nil {
		r.writeFailedFrame(cm)
		return cerr
	}
	return r.write(cm)
}

// writeFailedFrame preserves a failed run's last frame on the piped table path.
// On a TTY the frame is already on screen. The write is best-effort: the
// command's own error is what the caller must see.
func (r Runner) writeFailedFrame(cm CommandModel) {
	if r.format != FormatTable || r.tty {
		return
	}
	_, _ = io.WriteString(r.out, cm.View())
}

func (r Runner) write(cm CommandModel) error {
	switch r.format {
	case FormatTable:
		if r.tty {
			return nil // bubbletea already drew it
		}
		if _, err := io.WriteString(r.out, cm.View()); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	case FormatJSON:
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cm.Payload()); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	case FormatYAML:
		if err := yaml.NewEncoder(r.out).Encode(cm.Payload()); err != nil {
			return fmt.Errorf("encode yaml: %w", err)
		}
		return nil
	}
	return nil
}
```

Both switches cover every `Format` value with no `default`, satisfying `exhaustive`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/ -run 'TestParseFormat|TestRunner' -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/tui.go internal/cli/tui/tui_test.go
git commit -m "feat(cli/tui): CommandModel contract and format-aware Runner"
```

---

### Task 8: Message set and the Instant flow

**Files:**
- Create: `internal/cli/tui/flow/msg.go`, `internal/cli/tui/flow/instant.go`
- Test: `internal/cli/tui/flow/instant_test.go`

**Interfaces:**
- Consumes: `theme.Theme`, `theme.NewSpinner`, `theme.TickMsg`, `tui.CommandModel`.
- Produces: `func flow.NewInstant[T any](th theme.Theme, label string, fetch func() (T, error), view func(T, theme.Theme) string) *Instant[T]`, satisfying `tui.CommandModel`.

`fetch` takes no context: `tea.Cmd` is `func() tea.Msg` and accepts no arguments, and CLAUDE.md forbids storing a context in a struct. The caller's closure captures `cmd.Context()`.

- [ ] **Step 1: Write the failing test**

```go
package flow_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer
	return theme.New(lipgloss.NewRenderer(&buf))
}

// drain runs a tea.Cmd and feeds its message into the model.
func drain(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	require.NotNil(t, cmd)

	next, _ := m.Update(cmd())
	return next
}

func newCounter(th theme.Theme, fetch func() (int, error)) *flow.Instant[int] {
	return flow.NewInstant(th, "loading", fetch,
		func(n int, _ theme.Theme) string { return "count " + string(rune('0'+n)) + "\n" })
}

func TestInstant_Update_ReadyStoresDataAndQuits(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 7, nil })

	// Init batches the spinner tick and the fetch; run the fetch directly.
	cmds := m.Init()
	require.NotNil(t, cmds)

	next, quit := m.Update(fetchResult(t, m))
	model := next.(tui.CommandModel)

	require.NotNil(t, quit, "a terminal result must quit the program")
	assert.NoError(t, model.Err())
	assert.Equal(t, 7, model.Payload())
	assert.Equal(t, "count 7\n", model.View())
}

func TestInstant_Update_FetchErrorIsTerminal(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 0, errors.New("not found") })

	next, quit := m.Update(fetchResult(t, m))
	model := next.(tui.CommandModel)

	require.NotNil(t, quit)
	assert.ErrorContains(t, model.Err(), "not found")
	assert.Equal(t, "", model.View(), "a failed instant run renders nothing")
}

func TestInstant_View_SpinnerAppearsOnlyAfterDelay(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	assert.Equal(t, "", m.View(), "spinner must not flash")

	var model tea.Model = m
	for range 2 {
		model, _ = model.Update(theme.TickMsg(time.Now()))
	}

	assert.Contains(t, model.View(), "loading")
}

func TestInstant_Update_TicksStopOnceSettled(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	settled, _ := m.Update(fetchResult(t, m))
	_, cmd := settled.Update(theme.TickMsg(time.Now()))

	assert.Nil(t, cmd, "the tick chain must not be re-armed after settling")
}

func TestInstant_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = newCounter(newTestTheme(t), func() (int, error) { return 0, nil })
}
```

Add these two helpers to the same file. `runCmd` is reused by Tasks 10 and 11, so it belongs here:

```go
// runCmd executes cmd, unwrapping a tea.Batch and skipping spinner ticks.
// tea.BatchMsg is []tea.Cmd, so a batch must be run element by element.
func runCmd(cmd tea.Cmd) tea.Msg {
	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}

	for _, c := range batch {
		got := c()
		if got == nil {
			continue
		}
		if _, isTick := got.(theme.TickMsg); isTick {
			continue
		}
		return got
	}
	return nil
}

// fetchResult runs the model's Init command and returns the message it emits.
func fetchResult(t *testing.T, m tea.Model) tea.Msg {
	t.Helper()

	msg := runCmd(m.Init())
	require.NotNil(t, msg, "Init produced no non-tick message")
	return msg
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/flow/ -run TestInstant -v`
Expected: FAIL — `undefined: flow.NewInstant`.

- [ ] **Step 3: Write minimal implementation**

`msg.go`:

```go
// Package flow holds the three command lifecycles every CLI command runs on,
// plus the Confirm decorator.
package flow

type readyMsg[T any] struct{ data T }

type errMsg struct{ err error }
```

`instant.go`:

```go
package flow

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Instant is the flow for commands that resolve once, render, and quit.
type Instant[T any] struct {
	label string
	fetch func() (T, error)
	view  func(T, theme.Theme) string

	th      theme.Theme
	spin    theme.Spinner
	data    T
	err     error
	settled bool
}

// NewInstant returns a flow that runs fetch, renders it with view, and quits.
// fetch takes no context: the caller's closure captures it.
func NewInstant[T any](
	th theme.Theme,
	label string,
	fetch func() (T, error),
	view func(T, theme.Theme) string,
) *Instant[T] {
	return &Instant[T]{
		label: label,
		fetch: fetch,
		view:  view,
		th:    th,
		spin:  theme.NewSpinner(),
	}
}

// Init starts the spinner and the fetch.
func (m *Instant[T]) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick(), m.load())
}

func (m *Instant[T]) load() tea.Cmd {
	return func() tea.Msg {
		data, err := m.fetch()
		if err != nil {
			return errMsg{err: err}
		}
		return readyMsg[T]{data: data}
	}
}

// Update advances the spinner until a terminal message settles the flow.
func (m *Instant[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case theme.TickMsg:
		if m.settled {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case readyMsg[T]:
		m.data = msg.data
		m.settled = true
		return m, tea.Quit
	case errMsg:
		m.err = msg.err
		m.settled = true
		return m, tea.Quit
	}
	return m, nil
}

// View renders the spinner while loading and the result once settled.
func (m *Instant[T]) View() string {
	if !m.settled {
		frame := m.spin.Frame()
		if frame == "" {
			return ""
		}
		return frame + " " + m.th.Muted.Render(m.label) + "\n"
	}
	if m.err != nil {
		return ""
	}
	return m.view(m.data, m.th)
}

// Payload returns the fetched data.
func (m *Instant[T]) Payload() any { return m.data }

// Err returns the terminal error, or nil.
func (m *Instant[T]) Err() error { return m.err }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/flow/ -run TestInstant -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/flow/msg.go internal/cli/tui/flow/instant.go internal/cli/tui/flow/instant_test.go
git commit -m "feat(cli/tui): Instant flow with causally-bound spinner"
```

---

### Task 9: Transactional flow

**Files:**
- Create: `internal/cli/tui/flow/transactional.go`
- Test: `internal/cli/tui/flow/transactional_test.go`

**Interfaces:**
- Consumes: `flow.Instant` (Task 8).
- Produces: `type flow.Transactional[T any] = Instant[T]`; `flow.TxOpts[T any]{Label string; Do func() (T, error); View func(T, theme.Theme) string}`; `func flow.NewTransactional[T any](th theme.Theme, o TxOpts[T]) *Transactional[T]`.

`Transactional` is a generic type alias for `Instant` because their mechanics are identical — one round trip, one terminal result. The separate constructor names the intent at call sites and allows the two to diverge later without touching them.

- [ ] **Step 1: Write the failing test**

```go
func TestTransactional_Update_ReportsOutcome(t *testing.T) {
	th := newTestTheme(t)
	m := flow.NewTransactional(th, flow.TxOpts[string]{
		Label: "adding arrow",
		Do:    func() (string, error) { return "github.com/u/r", nil },
		View: func(ns string, vt theme.Theme) string {
			return component.Outcome(component.Result{OK: true, Subject: ns, Message: "added"}, vt)
		},
	})

	next, quit := m.Update(fetchResult(t, m))
	model := next.(tui.CommandModel)

	require.NotNil(t, quit)
	assert.NoError(t, model.Err())
	assert.Equal(t, "✓ github.com/u/r  added\n", model.View())
	assert.Equal(t, "github.com/u/r", model.Payload())
}

func TestTransactional_Update_MutationErrorIsTerminal(t *testing.T) {
	th := newTestTheme(t)
	m := flow.NewTransactional(th, flow.TxOpts[string]{
		Label: "adding arrow",
		Do:    func() (string, error) { return "", errors.New("already exists") },
		View:  func(string, theme.Theme) string { return "unreachable" },
	})

	next, _ := m.Update(fetchResult(t, m))
	model := next.(tui.CommandModel)

	assert.ErrorContains(t, model.Err(), "already exists")
	assert.Equal(t, "", model.View())
}

func TestTransactional_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = flow.NewTransactional(newTestTheme(t), flow.TxOpts[int]{
		Do:   func() (int, error) { return 0, nil },
		View: func(int, theme.Theme) string { return "" },
	})
}
```

Add `"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/flow/ -run TestTransactional -v`
Expected: FAIL — `undefined: flow.NewTransactional`.

- [ ] **Step 3: Write minimal implementation**

```go
package flow

import (
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Transactional is the flow for commands that mutate state and report an
// outcome. It shares Instant's engine: one round trip, one terminal result.
type Transactional[T any] = Instant[T]

// TxOpts configures a Transactional flow.
type TxOpts[T any] struct {
	// Label is shown beside the spinner while the mutation is in flight.
	Label string
	// Do performs the mutation. It takes no context: the caller's closure
	// captures it.
	Do func() (T, error)
	// View renders the outcome.
	View func(T, theme.Theme) string
}

// NewTransactional returns a flow that performs o.Do and reports its outcome.
func NewTransactional[T any](th theme.Theme, o TxOpts[T]) *Transactional[T] {
	return NewInstant(th, o.Label, o.Do, o.View)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/flow/ -run TestTransactional -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/flow/transactional.go internal/cli/tui/flow/transactional_test.go
git commit -m "feat(cli/tui): Transactional flow over the Instant engine"
```

---

### Task 10: Streaming flow

**Files:**
- Create: `internal/cli/tui/flow/streaming.go`
- Modify: `internal/cli/tui/flow/msg.go` (add `openedMsg`, `eventMsg`)
- Test: `internal/cli/tui/flow/streaming_test.go`

**Interfaces:**
- Consumes: `component.Step`, `component.StepState`, `theme.Theme`.
- Produces: `flow.EventKind` (`EventStep`, `EventDone`, `EventFailed`); `flow.Event[T any]{Kind EventKind; Name string; State component.StepState; Final *T; Err error}`; `flow.StreamOpts[T any]{Label string; Start func() (<-chan Event[T], error); View func([]component.Step, *T, theme.Theme) string}`; `func flow.NewStreaming[T any](th theme.Theme, o StreamOpts[T]) *Streaming[T]`.

`View`'s `final` is nil for every frame until the run reaches its terminal result, which is how the view knows to render `Steps` rather than `Outcome`.

- [ ] **Step 1: Write the failing test**

```go
func streamView(steps []component.Step, final *string, th theme.Theme) string {
	out := component.Steps(steps, th)
	if final != nil {
		out += component.Outcome(component.Result{OK: true, Subject: *final, Message: "done"}, th)
	}
	return out
}

func newStream(t *testing.T, events ...flow.Event[string]) *flow.Streaming[string] {
	t.Helper()

	ch := make(chan flow.Event[string], len(events))
	for _, e := range events {
		ch <- e
	}
	close(ch)

	return flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return ch, nil },
		View:  streamView,
	})
}

// pump drives the model until it quits, running each command it returns.
// The bound is a guard against a flow that never settles.
func pump(t *testing.T, m tea.Model) tui.CommandModel {
	t.Helper()

	cmd := m.Init()
	for range 100 {
		if cmd == nil {
			break
		}

		msg := runCmd(cmd)
		if msg == nil {
			break
		}
		if _, quit := msg.(tea.QuitMsg); quit {
			break
		}

		m, cmd = m.Update(msg)
	}

	cm, ok := m.(tui.CommandModel)
	require.True(t, ok, "model must satisfy tui.CommandModel")
	return cm
}

func TestStreaming_Run_RendersStepsThenOutcome(t *testing.T) {
	done := "github.com/u/r"
	m := newStream(t,
		flow.Event[string]{Kind: flow.EventStep, Name: "fetch source", State: component.StepDone},
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepRunning},
		flow.Event[string]{Kind: flow.EventDone, Final: &done},
	)

	model := pump(t, m)

	require.NoError(t, model.Err())
	view := model.View()
	assert.Contains(t, view, "fetch source")
	assert.Contains(t, view, "build")
	assert.Contains(t, view, "✓ github.com/u/r  done")
}

func TestStreaming_Run_FailureMarksStepAndKeepsFrame(t *testing.T) {
	m := newStream(t,
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepRunning},
		flow.Event[string]{Kind: flow.EventFailed, Err: errors.New("exit status 1")},
	)

	model := pump(t, m)

	require.ErrorContains(t, model.Err(), "exit status 1")
	assert.Contains(t, model.View(), "✗ 1 of 1  build",
		"the failing step must stay visible")
}

func TestStreaming_Run_StartErrorIsTerminal(t *testing.T) {
	m := flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return nil, errors.New("dial failed") },
		View:  streamView,
	})

	model := pump(t, m)

	assert.ErrorContains(t, model.Err(), "dial failed")
}

func TestStreaming_Run_ChannelClosedEarlyIsAnError(t *testing.T) {
	m := newStream(t) // no events, channel closed immediately

	model := pump(t, m)

	assert.ErrorContains(t, model.Err(), "stream closed before completion")
}

func TestStreaming_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = newStream(t)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/flow/ -run TestStreaming -v`
Expected: FAIL — `undefined: flow.NewStreaming`.

- [ ] **Step 3: Write minimal implementation**

Append to `msg.go`:

```go
type openedMsg[T any] struct{ ch <-chan Event[T] }

type eventMsg[T any] struct{ ev Event[T] }
```

`streaming.go`:

```go
package flow

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// EventKind is the category of a streamed event.
type EventKind int

const (
	// EventStep reports a step beginning, completing or failing.
	EventStep EventKind = iota
	// EventDone reports successful completion and carries the final payload.
	EventDone
	// EventFailed reports termination in failure.
	EventFailed
)

// Event is one message from a running lifecycle method.
type Event[T any] struct {
	Kind  EventKind
	Name  string
	State component.StepState
	Final *T
	Err   error
}

// StreamOpts configures a Streaming flow.
type StreamOpts[T any] struct {
	// Label is shown beside the spinner before the first step arrives.
	Label string
	// Start opens the event stream. It takes no context: the caller's closure
	// captures it.
	Start func() (<-chan Event[T], error)
	// View renders the run. final is nil until the terminal result arrives.
	View func(steps []component.Step, final *T, t theme.Theme) string
}

// Streaming is the flow for commands that report progress until a terminal
// result.
type Streaming[T any] struct {
	label string
	start func() (<-chan Event[T], error)
	view  func([]component.Step, *T, theme.Theme) string

	th      theme.Theme
	spin    theme.Spinner
	ch      <-chan Event[T]
	steps   []component.Step
	final   *T
	err     error
	settled bool
}

// NewStreaming returns a flow that renders streamed steps until completion.
func NewStreaming[T any](th theme.Theme, o StreamOpts[T]) *Streaming[T] {
	return &Streaming[T]{
		label: o.Label,
		start: o.Start,
		view:  o.View,
		th:    th,
		spin:  theme.NewSpinner(),
	}
}

// Init starts the spinner and opens the stream.
func (m *Streaming[T]) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick(), m.open())
}

func (m *Streaming[T]) open() tea.Cmd {
	return func() tea.Msg {
		ch, err := m.start()
		if err != nil {
			return errMsg{err: err}
		}
		return openedMsg[T]{ch: ch}
	}
}

func (m *Streaming[T]) next() tea.Cmd {
	ch := m.ch
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return errMsg{err: errors.New("stream closed before completion")}
		}
		return eventMsg[T]{ev: ev}
	}
}

// Update folds streamed events into the step list until a terminal event.
func (m *Streaming[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case theme.TickMsg:
		if m.settled {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case openedMsg[T]:
		m.ch = msg.ch
		return m, m.next()
	case eventMsg[T]:
		return m.apply(msg.ev)
	case errMsg:
		m.err = msg.err
		m.settled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Streaming[T]) apply(ev Event[T]) (tea.Model, tea.Cmd) {
	switch ev.Kind {
	case EventStep:
		m.upsert(ev.Name, ev.State)
		return m, m.next()
	case EventDone:
		m.final = ev.Final
		m.settled = true
		return m, tea.Quit
	case EventFailed:
		m.failRunning()
		m.err = ev.Err
		m.settled = true
		return m, tea.Quit
	}
	return m, m.next()
}

func (m *Streaming[T]) upsert(name string, state component.StepState) {
	for i := range m.steps {
		if m.steps[i].Name == name {
			m.steps[i].State = state
			return
		}
	}
	m.steps = append(m.steps, component.Step{Name: name, State: state})
}

func (m *Streaming[T]) failRunning() {
	for i := range m.steps {
		if m.steps[i].State == component.StepRunning {
			m.steps[i].State = component.StepFailed
		}
	}
}

// View renders the spinner before the first step, then the step list, then the
// outcome. A failed run keeps its frame so the trace of where it died survives.
func (m *Streaming[T]) View() string {
	if len(m.steps) == 0 && !m.settled {
		frame := m.spin.Frame()
		if frame == "" {
			return ""
		}
		return frame + " " + m.th.Muted.Render(m.label) + "\n"
	}
	return m.view(m.steps, m.final, m.th)
}

// Payload returns the final result, or nil if the run did not complete.
func (m *Streaming[T]) Payload() any {
	if m.final == nil {
		return nil
	}
	return *m.final
}

// Err returns the terminal error, or nil.
func (m *Streaming[T]) Err() error { return m.err }
```

Both `switch` statements cover every value of their named type with no `default`, satisfying `exhaustive`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/flow/ -run TestStreaming -v`
Expected: PASS. If `pump` loops without settling, the likeliest cause is a missing `m.next()` on a non-terminal branch of `apply`.

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/flow/streaming.go internal/cli/tui/flow/msg.go internal/cli/tui/flow/streaming_test.go
git commit -m "feat(cli/tui): Streaming flow with steps-to-outcome transition"
```

---

### Task 11: Confirm decorator

**Files:**
- Create: `internal/cli/tui/flow/confirm.go`
- Test: `internal/cli/tui/flow/confirm_test.go`

**Interfaces:**
- Consumes: `tui.CommandModel`, `tui.Usage`, `theme.Theme`.
- Produces: `func flow.Confirm(th theme.Theme, prompt string, next tui.CommandModel) tui.CommandModel`; `func flow.ConfirmGuard(tty, yes bool) error`.

`Confirm` is a decorator rather than a flow because `uninstall` both confirms and streams. Aborting is not a failure — it yields `ExitOK` with no mutation.

- [ ] **Step 1: Write the failing test**

```go
func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestConfirm_View_ShowsPromptBeforeDeciding(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "remove github.com/u/r?", newCounter(th, func() (int, error) { return 1, nil }))

	assert.Contains(t, m.View(), "remove github.com/u/r?")
	assert.Nil(t, m.Init(), "confirm must not start the inner model before deciding")
}

func TestConfirm_Update_AcceptDelegatesToNext(t *testing.T) {
	th := newTestTheme(t)
	inner := newCounter(th, func() (int, error) { return 4, nil })
	m := flow.Confirm(th, "proceed?", inner)

	accepted, cmd := m.Update(key("y"))
	require.NotNil(t, cmd, "accepting must start the inner model")

	settled, _ := accepted.Update(fetchResult(t, inner))
	model := settled.(tui.CommandModel)

	assert.NoError(t, model.Err())
	assert.Equal(t, 4, model.Payload())
}

func TestConfirm_Update_AbortIsSuccessWithNoPayload(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "proceed?", newCounter(th, func() (int, error) {
		t.Fatal("inner model must not run when aborted")
		return 0, nil
	}))

	aborted, cmd := m.Update(key("n"))
	model := aborted.(tui.CommandModel)

	require.NotNil(t, cmd, "aborting must quit")
	assert.NoError(t, model.Err(), "aborting is not a failure")
	assert.Nil(t, model.Payload())
	assert.Equal(t, "", model.View())
}

func TestConfirm_Update_IgnoresUnrelatedKeys(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "proceed?", newCounter(th, func() (int, error) { return 1, nil }))

	next, cmd := m.Update(key("q"))

	assert.Nil(t, cmd)
	assert.Contains(t, next.View(), "proceed?")
}

func TestConfirmGuard_RequiresYesWhenNotATTY(t *testing.T) {
	testCases := []struct {
		name    string
		tty     bool
		yes     bool
		wantErr bool
	}{
		{"tty prompts", true, false, false},
		{"yes skips the prompt", false, true, false},
		{"piped without yes is a usage error", false, false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := flow.ConfirmGuard(tc.tty, tc.yes)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/flow/ -run TestConfirm -v`
Expected: FAIL — `undefined: flow.Confirm`.

- [ ] **Step 3: Write minimal implementation**

```go
package flow

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

type confirm struct {
	prompt string
	next   tui.CommandModel
	th     theme.Theme

	decided bool
	aborted bool
}

// Confirm wraps next in a yes/no prompt, delegating to it once accepted.
// Aborting is not a failure: it yields no payload and no error.
func Confirm(th theme.Theme, prompt string, next tui.CommandModel) tui.CommandModel {
	return &confirm{prompt: prompt, next: next, th: th}
}

// Init renders the prompt without starting the inner model.
func (m *confirm) Init() tea.Cmd { return nil }

// Update handles the prompt, then delegates every message to the inner model.
func (m *confirm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.decided {
		return m.delegate(msg)
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch k.String() {
	case "y", "Y", "enter":
		m.decided = true
		return m, m.next.Init()
	case "n", "N", "esc", "ctrl+c":
		m.aborted = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *confirm) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.next.Update(msg)
	if cm, ok := updated.(tui.CommandModel); ok {
		m.next = cm
	}
	return m, cmd
}

// View renders the prompt, then whatever the inner model renders.
func (m *confirm) View() string {
	if m.aborted {
		return ""
	}
	if !m.decided {
		return m.th.Label.Render(m.prompt) + m.th.Muted.Render(" [y/N] ") + "\n"
	}
	return m.next.View()
}

// Payload returns the inner model's payload, or nil if aborted.
func (m *confirm) Payload() any {
	if m.aborted || !m.decided {
		return nil
	}
	return m.next.Payload()
}

// Err returns the inner model's error. Aborting is not an error.
func (m *confirm) Err() error {
	if m.aborted || !m.decided {
		return nil
	}
	return m.next.Err()
}

// ConfirmGuard reports whether confirmation is possible. Prompting requires a
// terminal, so a piped invocation must pass --yes rather than proceed silently.
func ConfirmGuard(tty, yes bool) error {
	if yes || tty {
		return nil
	}
	return tui.Usage("this command requires confirmation; pass --yes to proceed without a prompt")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/tui/flow/ -run TestConfirm -v`
Expected: PASS

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add internal/cli/tui/flow/confirm.go internal/cli/tui/flow/confirm_test.go
git commit -m "feat(cli/tui): Confirm decorator composable with any flow"
```

---

### Task 12: End-to-end verification across flows and formats

**Files:**
- Create: `internal/cli/tui/tui_e2e_test.go`

**Interfaces:**
- Consumes: everything built in Tasks 1–11.
- Produces: nothing. This task is proof that the framework composes.

- [ ] **Step 1: Write the failing test**

```go
package tui_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

type arrow struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	State     string `json:"state"     yaml:"state"`
}

func TestFramework_InstantAcrossEveryFormat(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	testCases := []struct {
		name   string
		format tui.Format
		want   string
	}{
		{
			name:   "piped table",
			format: tui.FormatTable,
			want:   "NAMESPACE       STATE\ngithub.com/u/r  ready\n",
		},
		{
			name:   "json",
			format: tui.FormatJSON,
			want:   "{\n  \"namespace\": \"github.com/u/r\",\n  \"state\": \"ready\"\n}\n",
		},
		{
			name:   "yaml",
			format: tui.FormatYAML,
			want:   "namespace: github.com/u/r\nstate: ready\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			r := tui.NewRunner(&buf, tc.format, false)

			m := flow.NewInstant(r.Theme(), "loading catalog",
				func() (arrow, error) {
					return arrow{Namespace: "github.com/u/r", State: "ready"}, nil
				},
				func(a arrow, th theme.Theme) string {
					return component.Table(
						[]component.Column{{Title: "NAMESPACE"}, {Title: "STATE"}},
						[][]string{{a.Namespace, a.State}},
						"no arrows yet", th)
				})

			require.NoError(t, r.Run(context.Background(), m))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

func TestFramework_StreamingRendersStepsThenOutcome(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer
	r := tui.NewRunner(&buf, tui.FormatTable, false)

	done := "github.com/u/r"
	ch := make(chan flow.Event[string], 3)
	ch <- flow.Event[string]{Kind: flow.EventStep, Name: "fetch source", State: component.StepDone}
	ch <- flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepDone}
	ch <- flow.Event[string]{Kind: flow.EventDone, Final: &done}
	close(ch)

	m := flow.NewStreaming(r.Theme(), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return ch, nil },
		View: func(steps []component.Step, final *string, th theme.Theme) string {
			out := component.Steps(steps, th)
			if final != nil {
				out += component.Outcome(
					component.Result{OK: true, Subject: *final, Message: "installed"}, th)
			}
			return out
		},
	})

	require.NoError(t, r.Run(context.Background(), m))

	assert.Equal(t, ""+
		"✓ 1 of 2  fetch source\n"+
		"✓ 2 of 2  build\n"+
		"✓ github.com/u/r  installed\n", buf.String())
}

func TestFramework_EmptyResultIsDistinguishableFromFiltered(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	cols := []component.Column{{Title: "NAMESPACE"}}
	th := tui.NewRunner(&bytes.Buffer{}, tui.FormatTable, false).Theme()

	empty := component.Table(cols, nil, "no arrows yet", th)
	filtered := component.Table(cols, nil, "no arrows match -F ffmpeg", th)

	assert.NotEqual(t, empty, filtered,
		"the standard exists to make these two cases distinguishable")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/tui/ -run TestFramework -v`
Expected: FAIL initially only if an earlier task is incomplete. If Tasks 1–11 are done, these should compile and pass — that is the point of the task. If any assertion fails, the composition is wrong, not the test.

- [ ] **Step 3: Fix whatever composition defect the test exposes**

No new production code should be needed. If a test fails, correct the defect in the relevant flow or component and re-run its own task's tests to confirm no regression.

- [ ] **Step 4: Run the full gate**

```bash
make fmt
make vet
make lint
go test ./internal/cli/tui/... -race -cover
```

Expected: all pass, coverage ≥ 95% for every package under `internal/cli/tui/`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/tui/tui_e2e_test.go
git commit -m "test(cli/tui): end-to-end verification across flows and formats"
```

---

## Follow-up (not this plan)

Two items from the spec belong to the migration plan, not this one, because
both live in command wiring rather than the framework:

1. **Wrapping cobra's argument errors as `usageError`** (spec §8). `tui.Usage`
   exists after Task 6, but attaching it requires custom `Args` validators on
   each cobra command. Until that is done, `quiver search` with no pattern
   still exits 1 rather than 2.
2. **Migrating the ~33 existing commands.** Per spec §12 the work is orderable
   by flow — all `Instant` commands, then `Transactional`, then `Streaming` —
   so each stage exercises one flow against many commands.

Both must be written with access to the existing command and client code, which
was deliberately out of bounds while designing this standard.
