package commands

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/lifecycle"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
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

// runMethod drives one method invocation: subscribe first so no step events
// are missed, fire the POST, then wait for the terminal event.
func (a *app) runMethod(cmd *cobra.Command, ns, op string, opts methodOpts) error {
	if err := validNS(ns); err != nil {
		return err
	}
	vars, err := parseData(opts.data)
	if err != nil {
		return err
	}
	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	if opts.detach {
		if _, err := cli.ExecuteMethod(cmd.Context(), ns, apiMethod(op), vars); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s initiated for %s\n", op, ns)
		return nil
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	events, err := cli.SubscribeRuntime(ctx, ns)
	if err != nil {
		return err
	}
	started, err := cli.ExecuteMethod(ctx, ns, apiMethod(op), vars)
	if err != nil {
		return err
	}
	if !started {
		// The daemon completed the request as an idempotent no-op: no runtime
		// events will stream, so stop waiting and report it instead of hanging.
		cancel()
		return a.reportNoOp(cmd, ns, op)
	}

	if a.deps.IsTTY() {
		return a.waitTTY(cmd, ns, op, events)
	}
	return a.waitPlain(ctx, cmd, ns, op, events)
}

// reportNoOp prints a success line for a method that had nothing to do.
func (a *app) reportNoOp(cmd *cobra.Command, ns, op string) error {
	msg := ns + ": " + noOpDetail(op)
	if a.deps.IsTTY() {
		msg = ui.Success.Render("✓") + "  " + msg
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), msg)
	return nil
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

// waitPlain renders line-per-transition progress for piped output.
func (a *app) waitPlain(
	ctx context.Context,
	cmd *cobra.Command,
	ns, op string,
	events <-chan apidto.ArrowRuntimeDTO,
) error {
	w := cmd.OutOrStdout()
	printer := lifecycle.NewPlainPrinter(w)
	res, err := lifecycle.Wait(ctx, events, op, printer.Observe)
	if err != nil {
		return err
	}
	if res.Outcome != "success" {
		_, _ = fmt.Fprintf(w, "%s %s: %s\n", op, ns, res.Outcome)
		if res.FailedStep != nil && res.FailedStep.Error != nil {
			_, _ = fmt.Fprintf(w, "  %s\n", *res.FailedStep.Error)
		}
		return fmt.Errorf("%s %s: %s", op, ns, res.Outcome)
	}
	_, _ = fmt.Fprintf(w, "%s %s: success (state %s)\n", op, ns, res.State)
	return nil
}

// waitTTY renders the BubbleTea lifecycle view inline.
func (a *app) waitTTY(
	cmd *cobra.Command,
	ns, op string,
	events <-chan apidto.ArrowRuntimeDTO,
) error {
	program := tea.NewProgram(
		lifecycle.NewModel(op, ns),
		tea.WithInput(cmd.InOrStdin()),
		tea.WithOutput(cmd.OutOrStdout()),
	)
	go func() {
		for evt := range events {
			program.Send(lifecycle.EventMsg(evt))
		}
		// Stream ended: unblock the program so a dropped connection does not
		// leave the spinner running forever.
		program.Quit()
	}()

	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("%s %s: render: %w", op, ns, err)
	}
	model := final.(lifecycle.Model)
	if !model.Done() {
		return fmt.Errorf("%s %s: interrupted", op, ns)
	}
	if model.Result().Outcome != "success" {
		return fmt.Errorf("%s %s: %s", op, ns, model.Result().Outcome)
	}
	return nil
}
