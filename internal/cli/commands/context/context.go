// Package context implements the `quiver context ...` commands: add, use,
// list, current, show, remove. These commands manage the local context
// store only — they never boot a daemon.
package context

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runner"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Commands builds the `quiver context` command subtree.
type Commands interface {
	Cmd() *cobra.Command
}

type commands struct {
	sess session.Session
	rb   runner.Builder
}

// New returns a Commands wired to sess and rb.
func New(
	sess session.Session,
	rb runner.Builder,
) Commands {
	return &commands{sess: sess, rb: rb}
}

func (c *commands) Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage named quiver.core instances",
	}
	cmd.AddCommand(
		c.addCmd(), c.useCmd(), c.listCmd(),
		c.currentCmd(), c.showCmd(), c.removeCmd(),
	)
	return cmd
}

func (c *commands) addCmd() *cobra.Command {
	var (
		server   string
		token    string
		insecure bool
		use      bool
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if server == "" {
				return tui.Usage("context add: --server is required")
			}
			cfg, err := c.sess.Config()
			if err != nil {
				return err
			}
			ctx := config.Context{
				Name: args[0], Server: server, Token: token, Insecure: insecure,
			}
			return invoke.RenderMutation(c.sess, c.rb, cmd, output.ActionAdd, args[0], func() error {
				return cfg.Add(ctx, use)
			})
		},
	}
	// A local --server shadows the root's persistent --server for this
	// subcommand, which is what a user means by
	// `context add <name> --server <uri>`. The root flag still targets a
	// daemon; here the value belongs to the context being created.
	cmd.Flags().StringVar(&server, "server", "", "server URI for the context (required)")
	cmd.Flags().StringVar(&server, "ctx-server", "", "deprecated alias for --server")
	_ = cmd.Flags().MarkHidden("ctx-server")
	cmd.Flags().StringVar(&token, "token", "", "bearer token for remote instances")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "skip TLS verification")
	cmd.Flags().BoolVar(&use, "use", false, "activate the context immediately")
	return cmd
}

func (c *commands) useCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.sess.Config()
			if err != nil {
				return err
			}
			return invoke.RenderMutation(c.sess, c.rb, cmd, output.ActionUse, args[0], func() error {
				return cfg.Use(args[0])
			})
		},
	}
}

// contextListDoc is the machine-readable context listing.
type contextListDoc struct {
	Active   string           `json:"active" yaml:"active"`
	Contexts []config.Context `json:"contexts" yaml:"contexts"`
}

func (c *commands) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke.RenderInstant(
				c.sess, c.rb, cmd, "loading contexts",
				func() (contextListDoc, error) {
					cfg, err := c.sess.Config()
					if err != nil {
						return contextListDoc{}, err
					}

					return contextListDoc{
						Active:   cfg.ActiveName(),
						Contexts: cfg.List(),
					}, nil
				},
				viewContextList,
			)
		},
	}
}

func (c *commands) currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return invoke.RenderInstant(
				c.sess, c.rb, cmd, "loading context",
				func() (config.Context, error) {
					cfg, err := c.sess.Config()
					if err != nil {
						return config.Context{}, err
					}

					return cfg.Active()
				},
				viewContext,
			)
		},
	}
}

func (c *commands) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show one context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return invoke.RenderInstant(
				c.sess, c.rb, cmd, "loading "+args[0],
				func() (config.Context, error) {
					cfg, err := c.sess.Config()
					if err != nil {
						return config.Context{}, err
					}

					return cfg.Get(args[0])
				},
				viewContext,
			)
		},
	}
}

func (c *commands) removeCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := c.sess.Config()
			if err != nil {
				return err
			}
			return invoke.RenderMutation(c.sess, c.rb, cmd, output.ActionRemove, args[0], func() error {
				return cfg.Remove(args[0], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "allow removing the active context")
	return cmd
}

func viewContextList(doc contextListDoc, t theme.Theme) string {
	rows := make([][]string, 0, len(doc.Contexts))

	for _, ctx := range doc.Contexts {
		// The active context is marked rather than sorted first, so the
		// listing order stays the order the contexts were added in.
		marker := " "
		if ctx.Name == doc.Active {
			marker = "*"
		}

		rows = append(rows, []string{marker, ctx.Name, ctx.Server})
	}

	return component.Table(
		[]component.Column{{Title: " "}, {Title: "NAME"}, {Title: "SERVER"}},
		rows, "no contexts", t,
	)
}

// field appends a labelled value, dropping it when empty so a detail view
// shows only what the subject actually has.
func field(fields []component.Field, label, value string) []component.Field {
	if value == "" {
		return fields
	}
	return append(fields, component.Field{Label: label, Value: value, Set: true})
}

func viewContext(ctx config.Context, t theme.Theme) string {
	var fields []component.Field

	fields = field(fields, "Name", ctx.Name)
	fields = field(fields, "Server", ctx.Server)

	// The token is deliberately not shown. It is a credential, and a detail
	// view is the easiest thing in the CLI to paste into a bug report.
	if ctx.Token != "" {
		fields = field(fields, "Token", "(set)")
	}

	if ctx.Insecure {
		fields = field(fields, "TLS", "insecure")
	}

	return component.Fields("CONTEXT", fields, t)
}
