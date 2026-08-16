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

// Event is one message from a running lifecycle method. The client package
// translates its wire DTOs into Event; flow does not import transport types.
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
	// View renders the run. final is nil until the terminal result arrives,
	// which is how the view knows to render Steps rather than Outcome.
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
