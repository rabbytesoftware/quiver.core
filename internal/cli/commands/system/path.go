package system

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
)

func (c *commands) pathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Report or configure whether ~/.quiver/bin is on PATH",
	}
	cmd.AddCommand(c.pathStatusCmd(), c.pathSetupCmd())
	return cmd
}

func (c *commands) pathStatusCmd() *cobra.Command {
	return c.pathSubCmd("status", "Report whether ~/.quiver/bin is on PATH", (*client.Client).PathStatus)
}

func (c *commands) pathSetupCmd() *cobra.Command {
	return c.pathSubCmd("setup", "Add ~/.quiver/bin to PATH", (*client.Client).SetupPath)
}

func (c *commands) pathSubCmd(
	use string,
	short string,
	call func(*client.Client, context.Context) (apidto.PathStatusDTO, error),
) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := c.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return err
			}

			status, err := call(cli, cmd.Context())
			if err != nil {
				return err
			}

			printPathStatus(cmd, status)
			return nil
		},
	}
}

func printPathStatus(
	cmd *cobra.Command,
	status apidto.PathStatusDTO,
) {
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "bin dir: %s\n", status.BinDir)
	_, _ = fmt.Fprintf(w, "on path: %s\n", yesNo(status.OnPath))
	_, _ = fmt.Fprintf(w, "configured: %s\n", yesNo(status.Configured))
	for _, file := range status.Files {
		_, _ = fmt.Fprintf(w, "file: %s\n", file)
	}
}

func yesNo(
	b bool,
) string {
	if b {
		return "yes"
	}
	return "no"
}
