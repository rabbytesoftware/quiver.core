package lifecycle

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

// EventMsg delivers one runtime WS event into the BubbleTea loop.
type EventMsg apidto.ArrowRuntimeDTO

// TickMsg drives the spinner and the elapsed-time display.
type TickMsg struct{}

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// Model is the TTY lifecycle view: header, "N of M · Xs" counter, the step
// list, and a completion box. It runs inline (no alt screen) so the final
// frame persists in terminal history.
type Model struct {
	op    string
	ns    string
	steps []apidto.StepProgressDTO

	start   time.Time
	elapsed time.Duration
	frame   int
	done    bool
	result  Result
}

// NewModel builds the view for one method execution.
func NewModel(op, ns string) Model {
	return Model{op: op, ns: ns, start: time.Now()}
}

// Done reports whether a terminal event arrived.
func (m Model) Done() bool { return m.done }

// Result returns the terminal outcome; valid once Done.
func (m Model) Result() Result { return m.result }

// Init schedules the first tick.
func (m Model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return TickMsg{} })
}

// Update consumes runtime events, ticks, and key presses.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TickMsg:
		m.elapsed = time.Since(m.start)
		m.frame++
		if m.done {
			return m, nil
		}
		return m, tick()

	case EventMsg:
		evt := apidto.ArrowRuntimeDTO(msg)
		if evt.ActiveRun != nil {
			m.steps = evt.ActiveRun.Steps
		}
		if res, isTerminal := terminal(evt, m.op); isTerminal {
			m.done = true
			m.result = res
			m.elapsed = time.Since(m.start)
			if len(res.Steps) > 0 {
				m.steps = res.Steps
			}
			return m, tea.Quit
		}
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the current frame.
func (m Model) View() string {
	var sb strings.Builder
	sb.WriteString(ui.CommandHeader(m.op, m.ns))
	sb.WriteString("    " + m.counter() + "\n\n")

	for _, s := range m.steps {
		sb.WriteString(m.stepLine(s))
	}
	if m.done {
		sb.WriteString("\n" + m.completionBox())
	}
	return sb.String()
}

func (m Model) counter() string {
	completed := 0
	for _, s := range m.steps {
		if s.Status == "completed" {
			completed++
		}
	}
	line := fmt.Sprintf("%d of %d", completed, len(m.steps))
	return ui.Muted.Render(line) + ui.Faint.Render(fmt.Sprintf("  ·  %.1fs", m.elapsed.Seconds()))
}

func (m Model) stepLine(s apidto.StepProgressDTO) string {
	num := ui.Faint.Render(fmt.Sprintf("%d", s.Index+1))
	title := StepTitle(s)
	if s.Title == "" {
		title = ui.Faint.Render("[Untitled step]")
	}

	var icon, name string
	switch s.Status {
	case "completed":
		icon = ui.Success.Render("✓")
		name = lipgloss.NewStyle().Foreground(ui.White).Render(title)
	case "running":
		icon = ui.Brand.Render(spinnerFrames[m.frame%len(spinnerFrames)])
		name = lipgloss.NewStyle().Foreground(ui.White).Render(title + "...")
	case "failed":
		icon = ui.Failure.Render("✗")
		name = ui.Failure.Render(title)
	default:
		icon = ui.Faint.Render("○")
		name = ui.Faint.Render(title)
	}

	line := fmt.Sprintf("    %s  %s  %s\n", num, icon, name)
	if s.Status == "failed" && s.Error != nil {
		line += "          " + ui.Failure.Render(*s.Error) + "\n"
	}
	return line
}

func (m Model) completionBox() string {
	var inner strings.Builder
	if m.result.Outcome == "success" {
		inner.WriteString(ui.Success.Bold(true).Render("✓  "+pastTense(m.op)) + "\n\n")
	} else {
		inner.WriteString(ui.Failure.Bold(true).Render("✗  "+m.op+" "+m.result.Outcome) + "\n\n")
	}

	kv := func(k, v string) {
		inner.WriteString(ui.Muted.Render(fmt.Sprintf("%-8s", k)) + "  " + v + "\n")
	}
	kv("Arrow", m.ns)
	if m.result.FailedStep != nil {
		kv("Failed", fmt.Sprintf("step %d · %s", m.result.FailedStep.Index+1, m.result.FailedStep.Type))
		if m.result.FailedStep.Error != nil {
			kv("Error", ui.Failure.Render(*m.result.FailedStep.Error))
		}
	}
	kv("State", ui.StateLabel(m.result.State))
	kv("Time", ui.Muted.Render(fmt.Sprintf("%.1fs", m.elapsed.Seconds())))

	return ui.RenderBox(strings.TrimRight(inner.String(), "\n"))
}

func pastTense(op string) string {
	switch op {
	case "install":
		return "Installed successfully"
	case "uninstall":
		return "Uninstalled successfully"
	case "update":
		return "Updated successfully"
	case "stop":
		return "Stopped successfully"
	case "run":
		return "Runtime started"
	default:
		return op + " completed"
	}
}
