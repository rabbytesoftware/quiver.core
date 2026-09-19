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

// Confirm wraps next in a yes/no prompt, delegating to it once accepted. It is
// a decorator rather than a flow because uninstall both confirms and streams.
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
