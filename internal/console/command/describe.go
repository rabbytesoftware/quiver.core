package command

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

func describe(
	root *cobra.Command,
) []CommandInfo {
	return collect(root, root, make([]CommandInfo, 0))
}

func collect(
	root *cobra.Command,
	parent *cobra.Command,
	infos []CommandInfo,
) []CommandInfo {
	for _, child := range parent.Commands() {
		infos = collectChild(root, child, infos)
	}
	return infos
}

func collectChild(
	root *cobra.Command,
	child *cobra.Command,
	infos []CommandInfo,
) []CommandInfo {
	if !clierr.IsConsole(child) || child.Hidden {
		return infos
	}
	if child.Runnable() {
		infos = append(infos, info(root, child))
	}
	return collect(root, child, infos)
}

func info(
	root *cobra.Command,
	cmd *cobra.Command,
) CommandInfo {
	return CommandInfo{
		Path:    strings.Fields(strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")),
		Short:   cmd.Short,
		Usage:   strings.TrimPrefix(cmd.UseLine(), root.Name()+" "),
		Aliases: append([]string{}, cmd.Aliases...),
		Flags:   flagInfos(cmd),
	}
}

func flagInfos(
	cmd *cobra.Command,
) []FlagInfo {
	denied := clierr.ConsoleDeniedFlags(cmd)
	flags := make([]FlagInfo, 0)
	cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		flags = appendFlag(flags, flag, denied)
	})
	return flags
}

func appendFlag(
	flags []FlagInfo,
	flag *pflag.Flag,
	denied []string,
) []FlagInfo {
	if flag.Hidden || flag.Name == "help" || slices.Contains(denied, flag.Name) {
		return flags
	}
	return append(flags, FlagInfo{
		Name:      flag.Name,
		Shorthand: flag.Shorthand,
		Usage:     flag.Usage,
		Takes:     flag.NoOptDefVal == "",
	})
}
