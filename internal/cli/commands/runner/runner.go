// Package runner builds the tui.Runner each command renders through, from
// the shared --output flag and whether stdout is a terminal.
package runner

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// Flags is the subset of global flags runner needs. The root command binds
// its --output flag into this struct's Output field.
type Flags struct {
	Output string
}

// Builder builds a tui.Runner for one command invocation.
type Builder interface {
	// Build returns the renderer for this invocation. An unset --output
	// means a table on a terminal and json when piped, so a redirected
	// command stays machine-readable without the caller asking.
	Build(cmd *cobra.Command, isTTY bool) (tui.Runner, error)
}

type builder struct {
	flags *Flags
}

// New returns a Builder reading Output from flags on every call, so a
// value bound to a cobra flag is picked up after parsing.
func New(flags *Flags) Builder {
	return &builder{flags: flags}
}

func (b *builder) Build(
	cmd *cobra.Command,
	isTTY bool,
) (tui.Runner, error) {
	name := b.flags.Output
	if name == "" {
		name = "table"
		if !isTTY {
			name = "json"
		}
	}

	format, err := tui.ParseFormat(name)
	if err != nil {
		return tui.Runner{}, err
	}

	return tui.NewRunner(
		cmd.OutOrStdout(), format, isTTY,
		tui.WithInput(cmd.InOrStdin()),
	), nil
}
