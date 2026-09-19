// Package collection implements the `quiver collection ...` commands:
// follow, unfollow, update, list, show, seed.
package collection

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

// Commands builds the `quiver collection` command subtree.
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
		Use:   "collection",
		Short: "Manage followed collections",
	}
	cmd.AddCommand(
		c.followCmd(), c.unfollowCmd(), c.updateCmd(),
		c.listCmd(), c.showCmd(), c.seedCmd(),
	)
	return cmd
}
