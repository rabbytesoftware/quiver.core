package commands

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// runInstant fetches one value from the daemon and renders it.
//
// These are free functions rather than methods on app because Go does not
// allow type parameters on methods, and the payload type has to travel from
// the fetch into the view.
func runInstant[T any](
	a *app,
	cmd *cobra.Command,
	label string,
	fetch func(*client.Client) (T, error),
	view func(T, theme.Theme) string,
) error {
	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	return renderInstant(a, cmd, label, func() (T, error) { return fetch(cli) }, view)
}

// renderInstant is runInstant without a daemon, for the commands that read
// only the local config.
func renderInstant[T any](
	a *app,
	cmd *cobra.Command,
	label string,
	fetch func() (T, error),
	view func(T, theme.Theme) string,
) error {
	runner, err := a.runner(cmd)
	if err != nil {
		return err
	}

	return runner.Run(
		cmd.Context(),
		flow.NewInstant(runner.Theme(), label, fetch, view),
	)
}

// ─── shared tables ───────────────────────────────────────────────────────────
//
// Every listing that shows the same entity shows it with the same columns, so
// `quiver arrow list` and the ARROWS section of `quiver list` cannot drift
// apart.

func runtimeColumns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "STATE"}, {Title: "METHOD"}, {Title: "PID"},
	}
}

func runtimeTable(runtimes []apidto.ArrowRuntimeDTO, empty string, t theme.Theme) string {
	rows := make([][]string, 0, len(runtimes))

	for _, rt := range runtimes {
		method, pid := "-", "-"
		if rt.ActiveRun != nil {
			method = rt.ActiveRun.Method
			if rt.ActiveRun.PID != 0 {
				pid = strconv.Itoa(rt.ActiveRun.PID)
			}
		}

		rows = append(rows, []string{
			rt.Namespace, t.State(domain.ArrowState(rt.State)), method, pid,
		})
	}

	return component.Table(runtimeColumns(), rows, empty, t)
}

// ─── shared detail views ─────────────────────────────────────────────────────

// field appends a labelled value, dropping it when empty so a detail view
// shows only what the subject actually has.
func field(fields []component.Field, label, value string) []component.Field {
	if value == "" {
		return fields
	}

	return append(fields, component.Field{Label: label, Value: value, Set: true})
}

func viewArrowDetail(d apidto.ArrowDetailDTO, t theme.Theme) string {
	var fields []component.Field

	fields = field(fields, "Namespace", d.Namespace)
	fields = field(fields, "Name", d.Name)
	fields = field(fields, "State", t.State(domain.ArrowState(d.State)))
	fields = field(fields, "Description", d.Description)
	fields = field(fields, "License", d.License)
	fields = field(fields, "Tags", strings.Join(d.Tags, ", "))
	fields = field(fields, "Constraint", d.InstalledConstraint)
	fields = field(fields, "Installed", d.InstalledAt)

	return component.Fields("ARROW", fields, t)
}

func viewMethods(methods []methodInfo, t theme.Theme) string {
	rows := make([][]string, 0, len(methods))

	for _, m := range methods {
		kind := "custom"
		if m.Builtin {
			kind = "built-in"
		}

		rows = append(rows, []string{m.Name, kind, strings.Join(m.AvailableIn, ", ")})
	}

	return component.Table(
		[]component.Column{
			{Title: "METHOD"}, {Title: "KIND"}, {Title: "AVAILABLE IN"},
		},
		rows, "no methods", t,
	)
}
