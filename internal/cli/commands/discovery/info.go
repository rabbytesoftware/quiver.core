package discovery

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// bareNS strips the @ref: manifest endpoints address arrows by bare namespace.
func bareNS(ns string) string {
	return domain.Namespace(ns).BareNamespace().String()
}

func (c *commands) infoCmd() *cobra.Command {
	var manifest bool
	cmd := &cobra.Command{
		Use:   "info <namespace>",
		Short: "Show an arrow's detail (or raw manifest)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			if manifest {
				return c.printManifest(cmd, args[0])
			}

			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.ArrowDetailDTO, error) {
					return cli.GetArrow(cmd.Context(), args[0])
				},
				arrow.ViewDetail,
			)
		},
	}
	cmd.Flags().BoolVar(&manifest, "manifest", false, "print the compiled manifest")
	return cmd
}

// printManifest writes the compiled manifest verbatim. It is the one output
// that is not a payload: the bytes are the answer, and re-encoding them
// through an output format would corrupt what the caller asked to see.
func (c *commands) printManifest(cmd *cobra.Command, ns string) error {
	cli, err := c.sess.Client(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	var raw json.RawMessage

	if err := c.sess.Spinner(cmd, "loading", func() error {
		var e error
		raw, e = cli.GetArrowManifest(cmd.Context(), bareNS(ns))

		return e
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(raw))

	return nil
}
