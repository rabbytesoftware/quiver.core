package clierr

import "github.com/spf13/cobra"

const annotationConsole = "quiver_console"

// AllowInConsole marks cmds as runnable from the daemon's console and returns
// them. The console is default-deny: a command is reachable only when it and
// every ancestor below the root carry this mark.
func AllowInConsole(cmds ...*cobra.Command) []*cobra.Command {
	for _, cmd := range cmds {
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[annotationConsole] = "true"
	}
	return cmds
}

// IsConsole reports whether cmd itself carries the console mark.
func IsConsole(cmd *cobra.Command) bool {
	return cmd.Annotations[annotationConsole] == "true"
}
