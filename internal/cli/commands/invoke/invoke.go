// Package invoke runs the two recurring command shapes — "fetch one thing
// and render it" and "perform one mutation and render its outcome" —
// against a session.Session and runner.Builder. These are free functions,
// not methods, because Go bars type parameters on methods and every
// command here is generic over its payload type.
package invoke

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// RunInstant fetches one value from the daemon and renders it.
func RunInstant[T any](
	sess session.Session,
	rb runner.Builder,
	cmd *cobra.Command,
	label string,
	fetch func(*client.Client) (T, error),
	view func(T, theme.Theme) string,
) error {
	cli, err := sess.Client(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	return RenderInstant(sess, rb, cmd, label, func() (T, error) { return fetch(cli) }, view)
}

// RenderInstant is RunInstant without a daemon, for commands that read
// only the local config.
func RenderInstant[T any](
	sess session.Session,
	rb runner.Builder,
	cmd *cobra.Command,
	label string,
	fetch func() (T, error),
	view func(T, theme.Theme) string,
) error {
	r, err := rb.Build(cmd, sess.IsTTY())
	if err != nil {
		return err
	}

	return r.Run(cmd.Context(), flow.NewInstant(r.Theme(), label, fetch, view))
}

// RunMutation performs a mutation against the daemon and renders its
// outcome.
func RunMutation(
	sess session.Session,
	rb runner.Builder,
	cmd *cobra.Command,
	action output.Action,
	subject string,
	do func(*client.Client) error,
) error {
	cli, err := sess.Client(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	return RenderMutation(sess, rb, cmd, action, subject, func() error { return do(cli) })
}

// RenderMutation is RunMutation without a daemon, for the context
// commands. They edit the local config file and must not boot a daemon to
// do it, which is why they cannot go through RunMutation.
func RenderMutation(
	sess session.Session,
	rb runner.Builder,
	cmd *cobra.Command,
	action output.Action,
	subject string,
	do func() error,
) error {
	r, err := rb.Build(cmd, sess.IsTTY())
	if err != nil {
		return err
	}

	model := flow.NewTransactional(r.Theme(), flow.TxOpts[output.Mutation]{
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

	return r.Run(cmd.Context(), model)
}

// viewMutation renders a mutation as the single line these commands have
// always printed, so a script reading stdout on the table path is
// unaffected by the payload it now carries.
func viewMutation(m output.Mutation, t theme.Theme) string {
	return t.OK.Render("✓") + " " + m.Action.Past() + " " + m.Subject + "\n"
}
