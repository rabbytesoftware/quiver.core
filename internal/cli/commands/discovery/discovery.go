// Package discovery implements the cross-resource commands: list, search,
// info, methods. It imports arrow and collection for their row-shaping
// and table-rendering functions rather than duplicating them, since these
// commands show both resources together.
package discovery

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

// Commands builds the list/search/info/methods commands.
type Commands interface {
	Cmd() []*cobra.Command
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

// Cmd returns the four cross-resource commands as root-level commands —
// none of them nest under a shared parent, the same reasoning as
// system.Commands and runtime.Commands.
func (c *commands) Cmd() []*cobra.Command {
	return []*cobra.Command{c.listCmd(), c.searchCmd(), c.infoCmd(), c.methodsCmd()}
}
