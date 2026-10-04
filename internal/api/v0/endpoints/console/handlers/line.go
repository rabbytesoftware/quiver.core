package console

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

const shellSyntax = "\"'`\\$;&|<>(){}*?~#"

var errBadLine = errors.New("line must be 1 to 1024 bytes of space-separated words, without control or shell characters; quoting is not supported")

// parseLine splits a console line into words. There is no quoting or escaping:
// a line holding a control character or anything a shell treats specially is
// refused, so no word can mean more than its own text.
func parseLine(line string) ([]string, error) {
	words := strings.Fields(line)
	if len(line) > 1024 || len(words) == 0 || strings.ContainsFunc(line, func(r rune) bool {
		return unicode.IsControl(r) || strings.ContainsRune(shellSyntax, r)
	}) {
		return nil, errBadLine
	}
	return words, nil
}

// allowed resolves args against root and returns the command to run, or an
// error naming the one refused. A command may run only when it and every
// ancestor below the root carry the console mark. The root never does, since
// its handler dispatches to arbitrary manifest methods.
func allowed(root *cobra.Command, args []string) (*cobra.Command, error) {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == root {
		return root, fmt.Errorf("command %q is not available in the console", args[0])
	}
	for c := cmd; c != root; c = c.Parent() {
		if !clierr.IsConsole(c) {
			return cmd, fmt.Errorf("command %q is not available in the console", strings.TrimPrefix(cmd.CommandPath(), "quiver "))
		}
	}
	return cmd, nil
}
