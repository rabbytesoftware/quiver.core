package command

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

type guard struct {
	approved *cobra.Command
}

func (g *guard) check(
	cmd *cobra.Command,
	_ []string,
) error {
	if cmd != g.approved {
		return &DeniedError{Reason: "unknown command"}
	}

	flag := deniedFlag(cmd)
	if flag == "" {
		return nil
	}
	return &DeniedError{
		Command: cmd.CommandPath(),
		Reason:  "the --" + flag + " flag is not available in the console",
	}
}

func deniedFlag(
	cmd *cobra.Command,
) string {
	names := append(clierr.ConsoleDeniedFlags(cmd), redirectFlags...)
	index := slices.IndexFunc(names, func(name string) bool {
		return flagChanged(cmd, name)
	})
	if index < 0 {
		return ""
	}
	return names[index]
}

func flagChanged(
	cmd *cobra.Command,
	name string,
) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}
