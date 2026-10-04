package command

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands"
)

type treeExecutor struct {
	opts Options
}

func (e *treeExecutor) Prepare(
	tokens []string,
	bearer string,
) (Invocation, error) {
	uri := e.opts.ServerURI
	if uri == "" {
		return nil, ErrUnavailable
	}

	if _, err := AuthorizeFlags(e.newRoot(&selfSession{}), tokens); err != nil {
		return nil, err
	}

	root := e.newRoot(&selfSession{uri: uri, token: bearer})
	approved, err := Authorize(root, tokens)
	if err != nil {
		return nil, err
	}

	g := &guard{approved: approved}
	root.PersistentPreRunE = g.check
	root.SetArgs(append([]string(nil), tokens...))
	return &preparedInvocation{root: root}, nil
}

func (e *treeExecutor) Commands() []CommandInfo {
	root := e.newRoot(&selfSession{})
	return describe(root)
}

func (e *treeExecutor) newRoot(
	sess *selfSession,
) *cobra.Command {
	root := &cobra.Command{
		Use:           "quiver",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(strings.NewReader(""))

	commands.New(commands.Deps{
		Version: e.opts.Version,
		Session: sess,
	}).Attach(root)

	_ = root.PersistentFlags().Set("output", "table")
	return root
}
