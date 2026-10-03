package runtime

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// methodOpts configures one lifecycle or custom method invocation.
type methodOpts struct {
	detach bool
	yes    bool
	data   []string
}

func (c *commands) lifecycleCmd(op, short string, confirmAction bool) *cobra.Command {
	opts := &methodOpts{}
	cmd := &cobra.Command{
		Use:   op + " <namespace>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirmAction {
				if err := clierr.Confirm(cmd, c.sess.IsTTY(), opts.yes, op+" "+args[0]); err != nil {
					return err
				}
			}
			return c.runMethod(cmd, args[0], op, *opts)
		},
	}
	cmd.Annotations = map[string]string{clierr.AnnotationLifecycle: "true"}
	cmd.Flags().BoolVar(&opts.detach, "detach", false, "fire the method without waiting")
	cmd.Flags().StringArrayVar(&opts.data, "data", nil, "method variable as key=value (repeatable)")
	if confirmAction {
		cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "skip the confirmation prompt")
	}
	return cmd
}

func (c *commands) installCmd() *cobra.Command {
	return c.lifecycleCmd("install", "Install an arrow and its dependencies", false)
}

func (c *commands) runCmd() *cobra.Command {
	return c.lifecycleCmd("run", "Execute an installed arrow", false)
}

func (c *commands) stopCmd() *cobra.Command {
	return c.lifecycleCmd("stop", "Stop a running arrow", false)
}

func (c *commands) uninstallCmd() *cobra.Command {
	return c.lifecycleCmd("uninstall", "Uninstall an arrow", true)
}

func (c *commands) updateCmd() *cobra.Command {
	return c.lifecycleCmd("update", "Update an arrow to the latest matching version", false)
}

// activateCmd applies the update an arrow staged. It never streams a run: the
// daemon restarts into the staged build instead of running steps, so the
// command reports the request and returns.
func (c *commands) activateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <namespace>",
		Short: "Restart the daemon to apply a staged update",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runMethod(cmd, args[0], "activate", methodOpts{})
		},
	}
	cmd.Annotations = map[string]string{clierr.AnnotationLifecycle: "true"}
	return cmd
}

// apiMethod maps a CLI operation to its runtime endpoint method.
func apiMethod(op string) string {
	if op == "run" {
		return "execute"
	}
	return op
}

// runMethod drives one method invocation.
func (c *commands) runMethod(cmd *cobra.Command, ns, op string, opts methodOpts) error {
	if err := clierr.ValidNS(ns); err != nil {
		return err
	}

	vars, err := clierr.ParseData(opts.data)
	if err != nil {
		return err
	}

	if opts.detach || op == "activate" {
		return c.fireAndForget(cmd, ns, op, vars)
	}

	return c.streamRun(cmd, ns, op, vars)
}

// fireAndForget starts the method and returns without waiting for it. The
// payload is a Mutation rather than a Run: no run was observed, so there are
// no steps and no outcome to report. A daemon that had nothing to do says so,
// and so does the command.
func (c *commands) fireAndForget(
	cmd *cobra.Command, ns, op string, vars map[string]string,
) error {
	cli, err := c.sess.Client(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	started, err := cli.ExecuteMethod(cmd.Context(), ns, apiMethod(op), vars)
	if err != nil {
		return err
	}
	if !started {
		return c.renderNoOp(cmd, ns, op)
	}

	return c.renderDetached(cmd, ns, op)
}

// noOpDetail describes why a method had nothing to do.
func noOpDetail(op string) string {
	switch op {
	case "install":
		return "already installed, nothing to do"
	case "update":
		return "already up to date, nothing to do"
	case "activate":
		return "nothing staged to activate"
	default:
		return "nothing to do"
	}
}

// detachedReason describes a method that was started without being waited on.
func detachedReason(op string) string {
	if op == "activate" {
		return "restarting to apply the update"
	}
	return "started, not waiting"
}

// renderDetached reports a method that was started without being waited on.
func (c *commands) renderDetached(cmd *cobra.Command, ns, op string) error {
	return invoke.RenderInstant(
		c.sess, c.rb, cmd, "",
		func() (output.NoOp, error) {
			return output.NoOp{
				Subject: ns,
				Method:  op,
				Reason:  detachedReason(op),
				At:      time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		func(n output.NoOp, t theme.Theme) string {
			return component.Outcome(component.Result{
				OK: true, Subject: n.Subject, Message: n.Reason,
			}, t)
		},
	)
}
