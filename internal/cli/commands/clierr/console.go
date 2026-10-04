package clierr

import (
	"strings"

	"github.com/spf13/cobra"
)

// AnnotationConsole marks a command the daemon's console may run. The console
// is default-deny: a command that does not carry this annotation, or whose
// parent chain up to the root does not, cannot be reached from it. The policy
// therefore lives next to each command's own definition rather than in a
// second list.
const AnnotationConsole = "quiver_console"

// AnnotationConsoleDeniedFlags lists, comma-separated, the flags of a console
// command the console must refuse even though the command itself is allowed.
const AnnotationConsoleDeniedFlags = "quiver_console_denied_flags"

// AllowInConsole marks cmd as runnable from the daemon's console and returns
// it, so a constructor can wrap its result.
func AllowInConsole(
	cmd *cobra.Command,
) *cobra.Command {
	setAnnotation(cmd, AnnotationConsole, "true")
	return cmd
}

// DenyInConsoleFlags makes the console refuse the named flags on cmd and
// returns it.
func DenyInConsoleFlags(
	cmd *cobra.Command,
	flags ...string,
) *cobra.Command {
	setAnnotation(cmd, AnnotationConsoleDeniedFlags, strings.Join(flags, ","))
	return cmd
}

// IsConsole reports whether cmd itself carries the console annotation. It does
// not look at cmd's ancestors; the console's policy does that.
func IsConsole(
	cmd *cobra.Command,
) bool {
	return cmd != nil && cmd.Annotations[AnnotationConsole] == "true"
}

// ConsoleDeniedFlags returns the flags the console refuses on cmd.
func ConsoleDeniedFlags(
	cmd *cobra.Command,
) []string {
	if cmd == nil || cmd.Annotations[AnnotationConsoleDeniedFlags] == "" {
		return nil
	}
	return strings.Split(cmd.Annotations[AnnotationConsoleDeniedFlags], ",")
}

func setAnnotation(
	cmd *cobra.Command,
	key string,
	value string,
) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[key] = value
}
