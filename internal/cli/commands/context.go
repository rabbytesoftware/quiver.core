package commands

import (
	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
)

func (a *app) contextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage named quiver.core instances",
	}
	cmd.AddCommand(
		a.contextAddCmd(), a.contextUseCmd(), a.contextListCmd(),
		a.contextCurrentCmd(), a.contextShowCmd(), a.contextRemoveCmd(),
	)
	return cmd
}

func (a *app) contextAddCmd() *cobra.Command {
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
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			ctx := config.Context{
				Name: args[0], Server: server, Token: token, Insecure: insecure,
			}
			return a.renderMutation(cmd, output.ActionAdd, args[0], func() error {
				return cfg.Add(ctx, use)
			})
		},
	}
	cmd.Flags().StringVar(&server, "ctx-server", "", "server URI for the context (required)")
	cmd.Flags().StringVar(&token, "token", "", "bearer token for remote instances")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "skip TLS verification")
	cmd.Flags().BoolVar(&use, "use", false, "activate the context immediately")
	_ = cmd.MarkFlagRequired("ctx-server")
	return cmd
}

func (a *app) contextUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			return a.renderMutation(cmd, output.ActionUse, args[0], func() error {
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

func (a *app) contextListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderInstant(
				a, cmd, "loading contexts",
				func() (contextListDoc, error) {
					cfg, err := a.loadConfig()
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

func (a *app) contextCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show the active context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderInstant(
				a, cmd, "loading context",
				func() (config.Context, error) {
					cfg, err := a.loadConfig()
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

func (a *app) contextShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show one context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderInstant(
				a, cmd, "loading "+args[0],
				func() (config.Context, error) {
					cfg, err := a.loadConfig()
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

func (a *app) contextRemoveCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := a.loadConfig()
			if err != nil {
				return err
			}
			return a.renderMutation(cmd, output.ActionRemove, args[0], func() error {
				return cfg.Remove(args[0], force)
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "allow removing the active context")
	return cmd
}
