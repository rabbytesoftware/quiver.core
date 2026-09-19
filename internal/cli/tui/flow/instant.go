package flow

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
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
// fetch takes no context: tea.Cmd accepts no arguments and a context must not
// be stored in a struct, so the caller's closure captures it.
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
	case tea.KeyMsg:
		if m.settled {
			return m, nil
		}
		if msg.Type != tea.KeyCtrlC {
			return m, nil
		}

		m.err = tui.Interrupted()
		m.settled = true

		return m, tea.Quit
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
