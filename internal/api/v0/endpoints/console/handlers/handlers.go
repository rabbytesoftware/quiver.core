package console

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

const maxRunning = 4

// Handlers serves the console endpoints: the log stream, the command list and
// command execution.
type Handlers struct {
	logs    logring.Ring
	version string
	slots   chan struct{}
	tree    func(session.Session) *cobra.Command
}

// New returns Handlers that stream the records of logs and run commands from
// the daemon's own CLI tree, which reports version as its own. At most four
// commands run at the same time across all callers.
func New(
	logs logring.Ring,
	version string,
) *Handlers {
	h := &Handlers{logs: logs, version: version, slots: make(chan struct{}, maxRunning)}
	h.tree = h.cliTree
	return h
}

func (h *Handlers) cliTree(
	sess session.Session,
) *cobra.Command {
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	commands.New(commands.Deps{Version: h.version, Session: sess}).Attach(root)
	_ = root.PersistentFlags().Set("output", "table")
	return root
}

func (h *Handlers) acquire() bool {
	select {
	case h.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (h *Handlers) release() {
	<-h.slots
}
