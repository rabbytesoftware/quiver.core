package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

// isActiveState reports whether a runtime deserves a row in plain ps.
func isActiveState(state string) bool {
	return state != "ready" && state != "absent" && state != "removed"
}

func runtimeRows(runtimes []apidto.ArrowRuntimeDTO) [][]string {
	rows := make([][]string, 0, len(runtimes))
	for _, rt := range runtimes {
		method, pid := "-", "-"
		if rt.ActiveRun != nil {
			method = rt.ActiveRun.Method
			if rt.ActiveRun.PID != 0 {
				pid = fmt.Sprintf("%d", rt.ActiveRun.PID)
			}
		}
		rows = append(rows, []string{
			rt.Namespace, ui.StateLabel(rt.State), method, pid,
		})
	}
	return rows
}

func writeRuntimeTable(w io.Writer, title string, runtimes []apidto.ArrowRuntimeDTO) {
	_, _ = fmt.Fprint(w, ui.CommandHeader(title, ""))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprint(w, ui.RenderTable(
		[]string{"NAMESPACE", "STATE", "METHOD", "PID"},
		runtimeRows(runtimes),
	))
}

func (a *app) psCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List active runtimes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			runtimes, err := cli.ListRuntimes(cmd.Context())
			if err != nil {
				return err
			}
			shown := make([]apidto.ArrowRuntimeDTO, 0, len(runtimes))
			for _, rt := range runtimes {
				if all || isActiveState(rt.State) {
					shown = append(shown, rt)
				}
			}
			return a.render(cmd, shown, func(w io.Writer) error {
				writeRuntimeTable(w, "ps", shown)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include idle arrows")
	return cmd
}

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [namespace]",
		Short: "Show runtime state for one arrow or all arrows",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				runtimes, err := cli.ListRuntimes(cmd.Context())
				if err != nil {
					return err
				}
				return a.render(cmd, runtimes, func(w io.Writer) error {
					writeRuntimeTable(w, "status", runtimes)
					return nil
				})
			}

			if err := validNS(args[0]); err != nil {
				return err
			}
			rt, err := cli.GetRuntime(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return a.render(cmd, rt, func(w io.Writer) error {
				writeRuntimeDetail(w, rt)
				return nil
			})
		},
	}
}

func writeRuntimeDetail(w io.Writer, rt apidto.ArrowRuntimeDTO) {
	_, _ = fmt.Fprint(w, ui.CommandHeader("status", rt.Namespace))
	_, _ = fmt.Fprintln(w)
	kv := func(k, v string) {
		_, _ = fmt.Fprintf(w, "  %s  %s\n", ui.Muted.Render(fmt.Sprintf("%-8s", k)), v)
	}
	kv("State", ui.StateLabel(rt.State))
	if rt.ActiveRun != nil {
		kv("Method", rt.ActiveRun.Method)
		if rt.ActiveRun.PID != 0 {
			kv("PID", fmt.Sprintf("%d", rt.ActiveRun.PID))
		}
	}
	if rt.LastReturn != nil {
		kv("Last run", rt.LastReturn.Method+" · "+rt.LastReturn.Outcome)
	}
}
