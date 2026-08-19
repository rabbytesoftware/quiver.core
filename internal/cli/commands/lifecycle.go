package commands

import (
	"time"

	"github.com/spf13/cobra"

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

func (a *app) lifecycleCmd(op, short string, confirmAction bool) *cobra.Command {
	opts := &methodOpts{}
	cmd := &cobra.Command{
		Use:   op + " <namespace>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if confirmAction {
				if err := a.confirm(cmd, opts.yes, op+" "+args[0]); err != nil {
					return err
				}
			}
			return a.runMethod(cmd, args[0], op, *opts)
		},
	}
	cmd.Annotations = map[string]string{AnnotationLifecycle: "true"}
	cmd.Flags().BoolVar(&opts.detach, "detach", false, "fire the method without waiting")
	cmd.Flags().StringArrayVar(&opts.data, "data", nil, "method variable as key=value (repeatable)")
	if confirmAction {
		cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "skip the confirmation prompt")
	}
	return cmd
}

func (a *app) installCmd() *cobra.Command {
	return a.lifecycleCmd("install", "Install an arrow and its dependencies", false)
}

func (a *app) runCmd() *cobra.Command {
	return a.lifecycleCmd("run", "Execute an installed arrow", false)
}

func (a *app) stopCmd() *cobra.Command {
	return a.lifecycleCmd("stop", "Stop a running arrow", false)
}

func (a *app) uninstallCmd() *cobra.Command {
	return a.lifecycleCmd("uninstall", "Uninstall an arrow", true)
}

func (a *app) updateCmd() *cobra.Command {
	return a.lifecycleCmd("update", "Update an arrow to the latest matching version", false)
}

// apiMethod maps a CLI operation to its runtime endpoint method.
func apiMethod(op string) string {
	if op == "run" {
		return "execute"
	}
	return op
}

// runMethod drives one method invocation.
func (a *app) runMethod(cmd *cobra.Command, ns, op string, opts methodOpts) error {
	if err := validNS(ns); err != nil {
		return err
	}

	vars, err := parseData(opts.data)
	if err != nil {
		return err
	}

	if opts.detach {
		return a.fireAndForget(cmd, ns, op, vars)
	}

	return a.streamRun(cmd, ns, op, vars)
}

// fireAndForget starts the method and returns without waiting for it. The
// payload is a Mutation rather than a Run: no run was observed, so there are
// no steps and no outcome to report.
func (a *app) fireAndForget(
	cmd *cobra.Command, ns, op string, vars map[string]string,
) error {
	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	if _, err := cli.ExecuteMethod(cmd.Context(), ns, apiMethod(op), vars); err != nil {
		return err
	}

	return a.renderDetached(cmd, ns, op)
}

// noOpDetail describes why a method had nothing to do.
func noOpDetail(op string) string {
	switch op {
	case "install":
		return "already installed, nothing to do"
	case "update":
		return "already up to date, nothing to do"
	default:
		return "nothing to do"
	}
}

// renderDetached reports a method that was started without being waited on.
func (a *app) renderDetached(cmd *cobra.Command, ns, op string) error {
	return renderInstant(
		a, cmd, "",
		func() (output.NoOp, error) {
			return output.NoOp{
				Subject: ns,
				Method:  op,
				Reason:  "started, not waiting",
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
