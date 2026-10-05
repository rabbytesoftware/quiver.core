package clierr

import "github.com/spf13/cobra"

const annotationConsole = "quiver_console"

// AllowInConsole marks every command in cmds as runnable from the daemon's
// console and returns them, so a constructor can wrap the commands it lists.
//
// The console is default-deny: a command is reachable only when it and every
// ancestor below the root carry this mark, so a command that is not passed here
// stays out of the console however it is reached.
func AllowInConsole(
	cmds ...*cobra.Command,
) []*cobra.Command {
	for _, cmd := range cmds {
		markConsole(cmd)
	}
	return cmds
}

// IsConsole reports whether cmd itself carries the console mark. It does not
// look at cmd's ancestors; the console's allow rule does that.
func IsConsole(
	cmd *cobra.Command,
) bool {
	return cmd.Annotations[annotationConsole] == "true"
}

func markConsole(
	cmd *cobra.Command,
) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[annotationConsole] = "true"
}
