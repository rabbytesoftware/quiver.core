package commands

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// renderMutation performs a mutation and renders its outcome, for the context
// commands. They edit the local config file and must not boot a daemon to do
// it, which is why they take a plain closure rather than going through
// session.
//
// On success the payload is an output.Mutation; on failure the Runner
// surfaces the error and writes no payload at all.
func (a *app) renderMutation(
	cmd *cobra.Command,
	action output.Action,
	subject string,
	do func() error,
) error {
	runner, err := a.runner(cmd)
	if err != nil {
		return err
	}

	model := flow.NewTransactional(runner.Theme(), flow.TxOpts[output.Mutation]{
		Label: string(action) + " " + subject,
		Do: func() (output.Mutation, error) {
			if doErr := do(); doErr != nil {
				return output.Mutation{}, doErr
			}

			return output.Mutation{
				Action:  action,
				Subject: subject,
				At:      time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		View: viewMutation,
	})

	return runner.Run(cmd.Context(), model)
}

// viewMutation renders a mutation as the single line these commands have
// always printed, so a script reading stdout on the table path is unaffected
// by the move to structured payloads.
func viewMutation(m output.Mutation, t theme.Theme) string {
	return t.OK.Render("✓") + " " + m.Action.Past() + " " + m.Subject + "\n"
}

// compile-time proof that the mutation flow satisfies the render contract.
var _ tui.CommandModel = (*flow.Transactional[output.Mutation])(nil)
