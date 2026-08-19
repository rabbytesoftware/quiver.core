package commands

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// runMutation performs a catalog mutation and renders its outcome.
//
// Every catalog mutation is the same shape of work — one round trip, one
// outcome — so they all run through here and differ only in the verb and the
// closure. On success the payload is an output.Mutation; on failure the
// Runner surfaces the error and writes no payload at all.
func (a *app) runMutation(
	cmd *cobra.Command,
	action output.Action,
	subject string,
	do func(*client.Client) error,
) error {
	runner, err := a.runner(cmd)
	if err != nil {
		return err
	}

	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	model := flow.NewTransactional(runner.Theme(), flow.TxOpts[output.Mutation]{
		Label: string(action) + " " + subject,
		Do: func() (output.Mutation, error) {
			if doErr := do(cli); doErr != nil {
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
