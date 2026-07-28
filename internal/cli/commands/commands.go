// Package commands assembles the quiver CLI command tree on top of the
// client, config, daemon, ui, and lifecycle packages. Attach registers every
// user-facing command on a root cobra command.
package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Deps injects the process-level collaborators commands need.
type Deps struct {
	// Version is the CLI build version shown by quiver version.
	Version string
	// IsTTY reports whether stdout is an interactive terminal.
	IsTTY func() bool
	// EnsureDaemon boots the local daemon when the resolved server is a Unix
	// socket. nil disables daemon management (remote contexts, tests).
	EnsureDaemon func(ctx context.Context) error
}

// app carries Deps plus the parsed global flags through every command.
type app struct {
	deps  Deps
	flags globalFlags
}

type globalFlags struct {
	server  string
	context string
	config  string
	output  string
}

// Attach registers the full CLI command surface on root.
func Attach(root *cobra.Command, d Deps) {
	a := &app{deps: d}

	pf := root.PersistentFlags()
	pf.StringVar(&a.flags.server, "server", "", "server URI (overrides context)")
	pf.StringVar(&a.flags.context, "context", "", "named context to target")
	pf.StringVar(&a.flags.config, "config", "", "path to the CLI config file")
	pf.StringVarP(&a.flags.output, "output", "o", "", "output format: table|json|yaml")

	root.Args = cobra.ArbitraryArgs
	root.RunE = a.dispatch
	root.Flags().Bool("detach", false, "fire the method without waiting")
	root.Flags().StringArray("data", nil, "method variable as key=value (repeatable)")

	root.AddCommand(
		a.installCmd(), a.runCmd(), a.stopCmd(), a.uninstallCmd(), a.updateCmd(),
		a.listCmd(), a.searchCmd(), a.infoCmd(), a.methodsCmd(),
		a.psCmd(), a.statusCmd(),
		a.arrowCmd(), a.collectionCmd(), a.contextCmd(),
		a.healthCmd(), a.versionCmd(),
	)
}

// usageError marks CLI misuse (bad arguments) for exit code 2.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usageErrorf(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// ExitCode maps a command error to the CLI process exit code.
func ExitCode(err error) int {
	var ue *usageError
	if errors.As(err, &ue) {
		return client.ExitUsage
	}
	return client.ExitCode(err)
}

// loadConfig opens the context store at --config or the default path.
func (a *app) loadConfig() (*config.Config, error) {
	path := a.flags.config
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	return config.Load(path)
}

// session resolves the target server and returns a connected client,
// booting the local daemon first when applicable.
func (a *app) session(cmd *cobra.Command) (*client.Client, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	server, err := cfg.Resolve(a.flags.server, a.flags.context)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(server, "unix://") && a.deps.EnsureDaemon != nil {
		if err := a.deps.EnsureDaemon(cmd.Context()); err != nil {
			return nil, err
		}
	}
	return client.New(server)
}

// render writes v in the effective output format; table falls back to the
// provided renderer.
func (a *app) render(cmd *cobra.Command, v any, table func(w io.Writer) error) error {
	format, err := ui.ResolveFormat(a.flags.output, a.deps.IsTTY())
	if err != nil {
		return usageErrorf("%v", err)
	}
	w := cmd.OutOrStdout()
	switch format {
	case ui.FormatJSON:
		return ui.WriteJSON(w, v)
	case ui.FormatYAML:
		return ui.WriteYAML(w, v)
	default:
		return table(w)
	}
}

// validNS checks that an argument is a plausible namespace.
func validNS(ns string) error {
	if err := domain.Namespace(ns).Validate(); err != nil {
		return usageErrorf("invalid namespace %q: %v", ns, err)
	}
	return nil
}

// confirm gates destructive commands: --force skips the prompt, a TTY asks,
// and a pipe without --force refuses.
func (a *app) confirm(cmd *cobra.Command, force bool, action string) error {
	if force {
		return nil
	}
	if !a.deps.IsTTY() {
		return usageErrorf("%s requires --force/-y when not running interactively", action)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s? [y/N] ", action)
	var answer string
	_, _ = fmt.Fscanln(cmd.InOrStdin(), &answer)
	if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
		return usageErrorf("%s cancelled", action)
	}
	return nil
}

// parseData turns repeated key=value flags into a variables map.
func parseData(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	vars := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found || key == "" {
			return nil, usageErrorf("invalid --data %q: expected key=value", pair)
		}
		vars[key] = value
	}
	return vars, nil
}
