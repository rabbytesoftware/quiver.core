// Package clierr holds the small, dependency-free primitives every CLI
// command package needs: usage-error validation, the confirmation prompt,
// exit-code mapping, and the lifecycle annotation main.go reads. It has no
// state and imports nothing from any command package, so every resource
// package (and the root) can depend on it without creating a cycle — the
// same role internal/api/libs plays for the API handler packages.
package clierr

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// AnnotationLifecycle marks a command that starts runtime work (install,
// run, stop, update, uninstall). The daemon must not be idle-stopped right
// after one of these, since it may still be executing.
const AnnotationLifecycle = "quiver_lifecycle"

// IsLifecycle reports whether cmd is a lifecycle-method command.
func IsLifecycle(cmd *cobra.Command) bool {
	return cmd != nil && cmd.Annotations[AnnotationLifecycle] == "true"
}

// ExitCode maps a command error to the CLI process exit code. tui.CodeFor
// classifies anything it recognizes; client.ExitCode covers the daemon
// connection/response errors tui does not know about.
func ExitCode(err error) int {
	if err == nil {
		return client.ExitOK
	}

	if code := tui.CodeFor(err); code != tui.ExitFailure {
		return code
	}

	return client.ExitCode(err)
}

// ValidNS checks that an argument is a plausible namespace.
func ValidNS(ns string) error {
	if err := domain.Namespace(ns).Validate(); err != nil {
		return tui.Usage("invalid namespace %q: %v", ns, err)
	}
	return nil
}

// ParseData turns repeated key=value flags into a variables map.
func ParseData(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	vars := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found || key == "" {
			return nil, tui.Usage("invalid --data %q: expected key=value", pair)
		}
		vars[key] = value
	}

	return vars, nil
}

// Confirm gates destructive commands: yes skips the prompt, an interactive
// terminal asks, and a pipe without yes refuses.
func Confirm(
	cmd *cobra.Command,
	isTTY bool,
	yes bool,
	action string,
) error {
	if yes {
		return nil
	}
	if !isTTY {
		return tui.Usage("%s requires --yes/-y when not running interactively", action)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s? [y/N] ", action)

	var answer string
	_, _ = fmt.Fscanln(cmd.InOrStdin(), &answer)

	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return tui.Usage("%s cancelled", action)
	}

	return nil
}
