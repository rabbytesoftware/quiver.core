package command

import "github.com/spf13/cobra"

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
