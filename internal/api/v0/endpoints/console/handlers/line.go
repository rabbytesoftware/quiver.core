package console

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

const (
	maxLine     = 1024
	shellSyntax = "\"'`\\$;&|<>(){}*?~#"
)

var errBadLine = errors.New("line must be 1 to 1024 bytes of space-separated words, without control or shell characters; quoting is not supported")

func parseLine(
	line string,
) ([]string, error) {
	words := strings.Fields(line)
	if len(line) > maxLine || len(words) == 0 || strings.ContainsFunc(line, isForbidden) {
		return nil, errBadLine
	}
	return words, nil
}

func isForbidden(
	r rune,
) bool {
	return unicode.IsControl(r) || strings.ContainsRune(shellSyntax, r)
}

func allowed(
	root *cobra.Command,
	args []string,
) (*cobra.Command, error) {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == root {
		return root, deniedError(args[0])
	}
	if !chainMarked(root, cmd) {
		return cmd, deniedError(strings.TrimPrefix(cmd.CommandPath(), "quiver "))
	}
	return cmd, nil
}

func chainMarked(
	root *cobra.Command,
	cmd *cobra.Command,
) bool {
	if cmd == root {
		return true
	}
	return clierr.IsConsole(cmd) && chainMarked(root, cmd.Parent())
}

func deniedError(
	name string,
) error {
	return fmt.Errorf("command %q is not available in the console", name)
}
