package command

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

func NewRootForTest(
	e Executor,
) *cobra.Command {
	return e.(*treeExecutor).newRoot(&selfSession{})
}

func NewInvocationForTest(
	root *cobra.Command,
) Invocation {
	return &preparedInvocation{root: root}
}

func NewSessionForTest(
	uri string,
	token string,
) session.Session {
	return &selfSession{uri: uri, token: token}
}

func CheckGuardForTest(
	approved *cobra.Command,
	cmd *cobra.Command,
) error {
	return (&guard{approved: approved}).check(cmd, nil)
}
