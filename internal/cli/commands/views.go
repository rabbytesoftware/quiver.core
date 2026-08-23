package commands

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/config"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
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

func arrowColumns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "REF"}, {Title: "STATE"},
	}
}

func collectionColumns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "ARROWS"},
	}
}

func runtimeColumns() []component.Column {
	return []component.Column{
		{Title: "NAMESPACE"}, {Title: "STATE"}, {Title: "METHOD"}, {Title: "PID"},
	}
}

func arrowTable(arrows []output.ArrowRow, t theme.Theme) string {
	return component.Table(arrowColumns(), arrowCatalogRows(arrows, t), "no arrows", t)
}

func collectionTable(collections []output.CollectionRow, t theme.Theme) string {
	return component.Table(
		collectionColumns(), collectionCatalogRows(collections), "no collections", t,
	)
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

// arrowRowsFrom shapes the API listing into the row type the tables share.
func arrowRowsFrom(arrows []apidto.ArrowListItemDTO) []output.ArrowRow {
	rows := make([]output.ArrowRow, 0, len(arrows))

	for _, arrow := range arrows {
		ref, state := installedRefAndState(arrow)
		rows = append(rows, output.ArrowRow{
			Namespace: arrow.Namespace,
			Name:      arrow.Name,
			Ref:       ref,
			State:     state,
		})
	}

	return rows
}

// collectionRowsFrom shapes the API listing into the row type the tables share.
func collectionRowsFrom(collections []apidto.CollectionListItemDTO) []output.CollectionRow {
	rows := make([]output.CollectionRow, 0, len(collections))

	for _, col := range collections {
		rows = append(rows, output.CollectionRow{
			Namespace: col.Namespace,
			Name:      col.Name,
			Arrows:    col.ArrowCount,
		})
	}

	return rows
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

func viewCollectionDetail(d apidto.CollectionDetailDTO, t theme.Theme) string {
	var fields []component.Field

	fields = field(fields, "Namespace", d.Namespace)
	fields = field(fields, "Name", d.Name)
	fields = field(fields, "Description", d.Description)

	rows := make([][]string, 0, len(d.Arrows))
	for _, arrow := range d.Arrows {
		resolved := "no"
		if arrow.Resolved {
			resolved = "yes"
		}

		rows = append(rows, []string{arrow.Namespace, arrow.Name, resolved})
	}

	return component.Fields("COLLECTION", fields, t) + "\n" +
		t.Header.Render("ARROWS") + "\n" +
		component.Table(
			[]component.Column{
				{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "RESOLVED"},
			},
			rows, "no arrows", t,
		)
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
