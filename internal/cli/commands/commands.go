// Package commands assembles the quiver CLI command tree. New wires every
// resource package (arrow, collection, context, system, runtime,
// discovery, auth) behind one Commands interface — the same shape
// engine/manifold uses for its resolver/translator/compiler/ruleset/hosts
// children: only this file imports all seven, and nothing outside this
// package reaches into a resource package directly.
package commands

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/auth"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/collection"
	cliContext "github.com/rabbytesoftware/quiver.core/internal/cli/commands/context"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/system"
)

// Deps injects the process-level collaborators the command tree needs.
type Deps struct {
	// Version is the CLI build version shown by quiver version.
	Version string
	// IsTTY reports whether stdout is an interactive terminal.
	IsTTY func() bool
	// EnsureDaemon boots the local daemon when the resolved server is a
	// Unix socket. nil disables daemon management (remote contexts, tests).
	EnsureDaemon func(ctx context.Context) error
}

// Commands attaches the full CLI command surface to a root command.
type Commands interface {
	Attach(root *cobra.Command)
}

type commandTree struct {
	sess       session.Session
	rb         runner.Builder
	flags      *globalFlags
	arrow      arrow.Commands
	collection collection.Commands
	context    cliContext.Commands
	system     system.Commands
	runtime    runtime.Commands
	discovery  discovery.Commands
	auth       auth.Commands
}

// globalFlags backs the root command's persistent flags. session.Flags and
// runner.Flags are embedded by pointer, not copied, so binding
// PersistentFlags onto them in Attach (which runs after New has already
// constructed every resource package around these same pointers) still
// reaches session and runner — cobra reads flag values lazily, at RunE
// time, never at bind time.
// New wires every resource package against one shared session.Session and
// runner.Builder, built from a single set of persistent flags bound onto
// root by Attach.
func New(deps Deps) Commands {
	flags := &globalFlags{}
	sess := session.New(session.Deps{
		Version:      deps.Version,
		IsTTYFunc:    deps.IsTTY,
		EnsureDaemon: deps.EnsureDaemon,
	}, &flags.session)
	rb := runner.New(&flags.runner)

	return &commandTree{
		sess:       sess,
		rb:         rb,
		arrow:      arrow.New(sess, rb),
		collection: collection.New(sess, rb),
		context:    cliContext.New(sess, rb),
		system:     system.New(sess, deps.Version),
		runtime:    runtime.New(sess, rb),
		discovery:  discovery.New(sess, rb),
		auth:       auth.New(sess, rb),
		flags:      flags,
	}
}

type globalFlags struct {
	session session.Flags
	runner  runner.Flags
}

func (t *commandTree) Attach(root *cobra.Command) {
	root.Long = strings.TrimSpace(`
Quiver installs and manages software through Git repositories ("arrows").

Run an arrow by naming it directly:

  quiver <namespace> [method]

With no method, "quiver <namespace>" prints everything you can do with that
arrow — its lifecycle actions and any custom methods from its manifest.`)
	root.Example = strings.TrimLeft(`
  quiver install github.com/user/repo      install an arrow
  quiver run     github.com/user/repo      run it
  quiver github.com/user/repo              show the arrow's action panel
  quiver github.com/user/repo <method>     run a custom manifest method`, "\n")

	pf := root.PersistentFlags()
	// The globalFlags struct embedded in commandTree is what session.Flags
	// and runner.Flags point at — binding here after New has already
	// constructed every resource package is safe because cobra reads flag
	// values lazily, at RunE time, not at bind time.
	pf.StringVar(&t.flags.session.Server, "server", "", "server URI (overrides context)")
	pf.StringVar(&t.flags.session.Context, "context", "", "named context to target")
	pf.StringVar(&t.flags.session.Config, "config", "", "path to the CLI config file")
	pf.StringVarP(&t.flags.runner.Output, "output", "o", "", "output format: table|json|yaml")

	root.Args = cobra.ArbitraryArgs
	root.RunE = t.dispatch
	root.Flags().Bool("detach", false, "fire the method without waiting")
	root.Flags().StringArray("data", nil, "method variable as key=value (repeatable)")

	root.AddCommand(t.runtime.Cmd()...)
	root.AddCommand(t.system.Cmd()...)
	root.AddCommand(t.discovery.Cmd()...)
	root.AddCommand(t.arrow.Cmd(), t.collection.Cmd(), t.context.Cmd(), t.auth.Cmd())
}

// ExitCode maps a command error to the CLI process exit code.
func ExitCode(err error) int { return clierr.ExitCode(err) }

// IsLifecycle reports whether cmd is a lifecycle-method command.
func IsLifecycle(cmd *cobra.Command) bool { return clierr.IsLifecycle(cmd) }

// IsActiveState reports whether an arrow state represents ongoing work.
func IsActiveState(state string) bool { return runtime.IsActiveState(state) }
