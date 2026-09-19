// Package auth implements `quiver auth devices list|revoke` — admin
// visibility over the device-pairing mechanism session uses automatically
// for tcp:// connections. There is no `auth pair`/`auth generate`: that
// round trip is internal to session, not a user-facing verb, since the
// whole point is that it happens without asking.
package auth

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

// Commands builds the `quiver auth` command subtree.
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
	devices := &cobra.Command{
		Use:   "devices",
		Short: "List or revoke paired devices",
	}
	devices.AddCommand(c.devicesListCmd(), c.devicesRevokeCmd())

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage devices paired with this daemon over tcp://",
	}
	cmd.AddCommand(devices)
	return cmd
}
