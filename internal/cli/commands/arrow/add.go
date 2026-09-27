package arrow

import (
	"errors"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

const addConfirmPrompt = "this arrow was auto-generated with low confidence. Install anyway"

func (c *commands) addCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add <namespace>",
		Short: "Register an arrow in the catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}

			cli, err := c.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return err
			}

			if err := addWithConfirm(cmd, cli, c.sess.IsTTY(), yes, args[0]); err != nil {
				return err
			}

			return invoke.RenderMutation(c.sess, c.rb, cmd, output.ActionAdd, args[0],
				func() error { return nil })
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the low-confidence confirmation prompt")
	return cmd
}

func addWithConfirm(
	cmd *cobra.Command,
	cli *client.Client,
	isTTY bool,
	yes bool,
	ns string,
) error {
	err := cli.AddArrow(cmd.Context(), ns, false)
	if !needsConfirmation(err) {
		return err
	}

	if confirmErr := clierr.Confirm(cmd, isTTY, yes, addConfirmPrompt); confirmErr != nil {
		return confirmErr
	}

	return cli.AddArrow(cmd.Context(), ns, true)
}

func needsConfirmation(
	err error,
) bool {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusConflict && strings.Contains(apiErr.Message, "confirmation required")
}
