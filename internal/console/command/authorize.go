package command

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

// Authorize resolves tokens against root and returns the one command the
// console may run for them, or a *DeniedError.
//
// A command is reachable only when it, and every ancestor up to root, carries
// the console annotation, it is not hidden and it is runnable. The root itself
// is never reachable: its own handler dispatches "<namespace> [method]" to
// arbitrary manifest methods, which the console does not expose.
//
// Before cobra is consulted, any token naming --server, --context or --config,
// in any spelling cobra accepts, is refused: those flags redirect a command to
// another server or another configuration file.
func Authorize(
	root *cobra.Command,
	tokens []string,
) (*cobra.Command, error) {
	found, _, err := resolve(root, tokens)
	return found, err
}

// AuthorizeFlags is Authorize plus a check of the parsed flags: it refuses a
// command whose invocation sets a flag the console denies for it, such as
// --watch on status, or any of the redirecting flags.
//
// It parses flags on root, so root must not be executed afterwards; the
// executor probes a throwaway tree with it and runs another.
func AuthorizeFlags(
	root *cobra.Command,
	tokens []string,
) (*cobra.Command, error) {
	found, rest, err := resolve(root, tokens)
	if err != nil {
		return nil, err
	}
	if err := found.ParseFlags(rest); err != nil {
		return found, nil
	}
	if flag := deniedFlag(found); flag != "" {
		return nil, flagDenied(found, flag)
	}
	return found, nil
}

func resolve(
	root *cobra.Command,
	tokens []string,
) (*cobra.Command, []string, error) {
	if flag := findRedirectFlag(tokens); flag != "" {
		return nil, nil, &DeniedError{Reason: "the " + flag + " flag is not available in the console"}
	}

	found, rest, err := root.Find(tokens)
	if err != nil {
		return nil, nil, &DeniedError{Reason: "unknown command"}
	}

	approved, err := approve(root, found)
	return approved, rest, err
}

func approve(
	root *cobra.Command,
	found *cobra.Command,
) (*cobra.Command, error) {
	path := found.CommandPath()
	if found == root {
		return nil, &DeniedError{Reason: "unknown command"}
	}
	if found.Hidden {
		return nil, &DeniedError{Command: path, Reason: "the command " + quote(path) + " is not available in the console"}
	}
	if !found.Runnable() {
		return nil, &DeniedError{Command: path, Reason: quote(path) + " needs a subcommand"}
	}
	if !chainAllowed(root, found) {
		return nil, &DeniedError{Command: path, Reason: "the command " + quote(path) + " is not available in the console"}
	}
	return found, nil
}

func chainAllowed(
	root *cobra.Command,
	cmd *cobra.Command,
) bool {
	if cmd == nil || cmd == root {
		return true
	}
	return clierr.IsConsole(cmd) && chainAllowed(root, cmd.Parent())
}

func findRedirectFlag(
	tokens []string,
) string {
	index := slices.IndexFunc(tokens, func(token string) bool {
		return redirectFlagName(token) != ""
	})
	if index < 0 {
		return ""
	}
	return redirectFlagName(tokens[index])
}

func redirectFlagName(
	token string,
) string {
	if !strings.HasPrefix(token, "--") {
		return ""
	}

	name, _, _ := strings.Cut(strings.TrimPrefix(token, "--"), "=")
	if !slices.Contains(redirectFlags(), name) {
		return ""
	}
	return "--" + name
}

func quote(
	path string,
) string {
	return "\"" + strings.TrimPrefix(path, "quiver ") + "\""
}

func redirectFlags() []string {
	return []string{"server", "context", "config"}
}
