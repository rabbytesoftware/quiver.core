package system

import (
	"fmt"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
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
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether ~/.quiver/bin is on PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := c.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return err
			}

			status, err := cli.PathStatus(cmd.Context())
			if err != nil {
				return err
			}

			printPathStatus(cmd, status)
			return nil
		},
	}
}

func (c *commands) pathSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Add ~/.quiver/bin to PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := c.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return err
			}

			status, err := cli.SetupPath(cmd.Context())
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
