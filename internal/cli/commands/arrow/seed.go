package arrow

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

func (c *commands) seedCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "seed <namespace>",
		Short: "Register an arrow from a local or piped manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			manifest, err := readManifest(cmd, file)
			if err != nil {
				return err
			}
			return invoke.RunMutation(c.sess, c.rb, cmd, output.ActionAdd, args[0],
				func(cli *client.Client) error {
					return cli.SeedArrowManifest(cmd.Context(), args[0], manifest)
				})
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "manifest file path, or - for stdin (required)")
	return cmd
}

// readManifest reads --file, or stdin when file is "-".
func readManifest(cmd *cobra.Command, file string) ([]byte, error) {
	if file == "" {
		return nil, tui.Usage("seed: --file is required (path, or - for stdin)")
	}
	if file == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(file) // #nosec G304 -- path is the file the user named via --file
}
