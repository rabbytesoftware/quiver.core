package theme

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TickMsg advances the spinner one frame.
type TickMsg time.Time

const (
	spinnerFPS = 80 * time.Millisecond
	// spinnerMinTicks suppresses the flash on fast commands: at spinnerFPS
	// this is roughly 160ms of silence before the spinner appears.
	spinnerMinTicks = 2
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

// Tick returns the command that schedules the next frame. It must be armed
// exactly once, from a model's Init: arming it again from Update would double
// the chain and the frame rate.
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
