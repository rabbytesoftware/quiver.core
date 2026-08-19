package commands

import (
	"strconv"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// IsActiveState reports whether an arrow state represents ongoing work —
// the states shown by plain ps and the ones that keep the local daemon alive.
func IsActiveState(state string) bool {
	switch state {
	case "running", "stopping", "draining", "detached",
		"installing", "updating", "uninstalling":
		return true
	default:
		return false
	}
}

func (a *app) psCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List active runtimes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstant(
				a, cmd, "loading runtimes",
				func(cli *client.Client) ([]apidto.ArrowRuntimeDTO, error) {
					runtimes, err := cli.ListRuntimes(cmd.Context())
					if err != nil {
						return nil, err
					}

					shown := make([]apidto.ArrowRuntimeDTO, 0, len(runtimes))
					for _, rt := range runtimes {
						if all || IsActiveState(rt.State) {
							shown = append(shown, rt)
						}
					}

					return shown, nil
				},
				viewRuntimeList("nothing running"),
			)
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
			// With no namespace this is the same listing as ps, unfiltered.
			if len(args) == 0 {
				return runInstant(
					a, cmd, "loading runtimes",
					func(cli *client.Client) ([]apidto.ArrowRuntimeDTO, error) {
						return cli.ListRuntimes(cmd.Context())
					},
					viewRuntimeList("no runtimes"),
				)
			}

			if err := validNS(args[0]); err != nil {
				return err
			}

			return runInstant(
				a, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.ArrowRuntimeDTO, error) {
					return cli.GetRuntime(cmd.Context(), args[0])
				},
				viewRuntimeDetail,
			)
		},
	}
}

// viewRuntimeList binds the empty-state wording to the table, so ps and status
// can say different things about an empty result while sharing the columns.
func viewRuntimeList(empty string) func([]apidto.ArrowRuntimeDTO, theme.Theme) string {
	return func(runtimes []apidto.ArrowRuntimeDTO, t theme.Theme) string {
		return runtimeTable(runtimes, empty, t)
	}
}

func viewRuntimeDetail(rt apidto.ArrowRuntimeDTO, t theme.Theme) string {
	var fields []component.Field

	fields = field(fields, "Namespace", rt.Namespace)
	fields = field(fields, "State", t.State(domain.ArrowState(rt.State)))

	if rt.ActiveRun != nil {
		fields = field(fields, "Method", rt.ActiveRun.Method)

		if rt.ActiveRun.PID != 0 {
			fields = field(fields, "PID", strconv.Itoa(rt.ActiveRun.PID))
		}
	}

	if rt.LastReturn != nil {
		fields = field(fields, "Last run",
			rt.LastReturn.Method+" · "+rt.LastReturn.Outcome)
	}

	out := component.Fields("RUNTIME", fields, t)

	// The steps of the active run are the answer to "what is it doing right
	// now", which the field list alone cannot show.
	if rt.ActiveRun != nil && len(rt.ActiveRun.Steps) > 0 {
		out += "\n" + t.Header.Render("STEPS") + "\n" + stepTable(rt.ActiveRun.Steps, t)
	}

	return out
}

func stepTable(steps []apidto.StepProgressDTO, t theme.Theme) string {
	rows := make([][]string, 0, len(steps))

	for _, s := range steps {
		detail := s.Title
		if s.Error != nil && *s.Error != "" {
			detail = *s.Error
		}

		rows = append(rows, []string{strconv.Itoa(s.Index), s.Status, detail})
	}

	return component.Table(
		[]component.Column{
			{Title: "#"}, {Title: "STATUS"}, {Title: "DETAIL"},
		},
		rows, "no steps", t,
	)
}
