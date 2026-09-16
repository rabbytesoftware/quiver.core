// Package arrow implements the `quiver arrow ...` catalog commands: add,
// remove, refresh, list, show, seed.
package arrow

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

// Commands builds the `quiver arrow` command subtree.
type Commands interface {
	Cmd() *cobra.Command
}

type commands struct {
	sess session.Session
	rb   runner.Builder
}

// New returns a Commands wired to sess and rb.
func New(
	sess session.Session,
	rb runner.Builder,
) Commands {
	return &commands{sess: sess, rb: rb}
}

func (c *commands) Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "arrow",
		Short: "Manage the arrow catalog",
	}
	cmd.AddCommand(
		c.addCmd(), c.removeCmd(), c.refreshCmd(),
		c.listCmd(), c.showCmd(), c.seedCmd(),
	)
	return cmd
}
