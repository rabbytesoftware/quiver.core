// Package runtime implements the lifecycle and observation commands:
// install, run, stop, uninstall, update, ps, status, watch.
package runtime

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

// Commands builds the install/run/stop/uninstall/update/ps/status/watch
// commands.
type Commands interface {
	Cmd() []*cobra.Command
	// RunMethod executes one method — a lifecycle verb or a manifest-defined
	// custom one — against ns and renders the outcome. It backs the root
	// command's bare `quiver <namespace> <method>` dispatch, which has no
	// cobra command of its own to hang a manifest-defined method on, yet
	// still needs the exact install/run/stop streaming machinery every
	// lifecycle command in this package already uses.
	RunMethod(cmd *cobra.Command, ns, op string, detach bool, data []string) error
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

// Cmd returns the eight lifecycle and observation commands as root-level
// commands — none of them nest under a shared parent, the same reasoning
// as system.Commands.
func (c *commands) Cmd() []*cobra.Command {
	return []*cobra.Command{
		c.installCmd(), c.runCmd(), c.stopCmd(), c.uninstallCmd(), c.updateCmd(),
		c.psCmd(), c.statusCmd(), c.watchCmd(),
	}
}

// RunMethod implements Commands.
func (c *commands) RunMethod(
	cmd *cobra.Command,
	ns, op string,
	detach bool,
	data []string,
) error {
	return c.runMethod(cmd, ns, op, methodOpts{detach: detach, data: data})
}
