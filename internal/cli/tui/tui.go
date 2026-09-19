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
	in     io.Reader
	theme  theme.Theme
}

// Option configures a Runner.
type Option func(*Runner)

// WithInput gives the Runner the reader interactive models read keys from.
//
// Without it bubbletea falls back to the process stdin and, failing that,
// opens /dev/tty directly. That reach-around makes the interactive path
// untestable and crashes wherever no controlling terminal exists, so the
// input is injected like every other stream the CLI touches.
func WithInput(in io.Reader) Option {
	return func(r *Runner) { r.in = in }
}

// NewRunner returns a Runner writing to out. The lipgloss renderer is bound to
// out so colour-profile detection matches the real destination.
func NewRunner(out io.Writer, format Format, tty bool, opts ...Option) Runner {
	r := Runner{
		format: format,
		tty:    tty,
		out:    out,
		theme:  theme.New(lipgloss.NewRenderer(out)),
	}

	for _, opt := range opts {
		opt(&r)
	}

	return r
}

// Theme returns the theme command models must render with.
func (r Runner) Theme() theme.Theme { return r.theme }

// Run executes m and writes its result in the configured format.
func (r Runner) Run(ctx context.Context, m CommandModel) error {
	opts := []tea.ProgramOption{tea.WithContext(ctx), tea.WithOutput(r.out)}
	if !r.tty || r.format != FormatTable {
		// No renderer and no input: nothing is drawn and nothing reads keys.
		// The reader must be nil rather than r.in, because bubbletea wraps a
		// non-nil input in a cancelreader, which an in-memory reader cannot
		// back.
		opts = append(opts, tea.WithoutRenderer(), tea.WithInput(nil))
	} else {
		// Interactive. Passing the input explicitly stops bubbletea falling
		// back to the process stdin and then to /dev/tty, a reach-around that
		// makes this path untestable and fails where no controlling terminal
		// exists.
		opts = append(opts, tea.WithInput(r.in))
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
		// yaml.v3 panics rather than erroring on a type it cannot marshal.
		// That is a programmer error in the command, caught by CheckPayload in
		// the command's own test and, in the last resort, by the panic barrier
		// in main. It is deliberately not recovered here.
		if err := yaml.NewEncoder(r.out).Encode(cm.Payload()); err != nil {
			return fmt.Errorf("encode yaml: %w", err)
		}

		return nil
	}

	return nil
}
