package commands

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// runCatalog fetches the catalog, filters it by pattern, and renders it.
//
// list and search differ only in how the pattern arrives — a --filter flag
// versus a positional argument — so they share everything below. The payload
// carries Query, which is what tells the two apart after the fact.
func (a *app) runCatalog(cmd *cobra.Command, pattern string) error {
	runner, err := a.runner(cmd)
	if err != nil {
		return err
	}

	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	model := flow.NewInstant(
		runner.Theme(),
		"loading catalog",
		func() (output.Catalog, error) {
			arrows, listErr := cli.ListArrows(ctx, nil)
			if listErr != nil {
				return output.Catalog{}, listErr
			}

			collections, colErr := cli.ListCollections(ctx)
			if colErr != nil {
				return output.Catalog{}, colErr
			}

			return buildCatalog(arrows, collections, pattern), nil
		},
		viewCatalog,
	)

	return runner.Run(ctx, model)
}

// buildCatalog filters the two listings by pattern and shapes them into the
// payload. An empty pattern matches everything.
func buildCatalog(
	arrows []apidto.ArrowListItemDTO,
	collections []apidto.CollectionListItemDTO,
	pattern string,
) output.Catalog {
	rows := make([]output.ArrowRow, 0, len(arrows))

	for _, arrow := range arrows {
		if !matches(pattern, arrow.Namespace, arrow.Name) {
			continue
		}

		ref, state := installedRefAndState(arrow)
		rows = append(rows, output.ArrowRow{
			Namespace: arrow.Namespace,
			Name:      arrow.Name,
			Ref:       ref,
			State:     state,
		})
	}

	cols := make([]output.CollectionRow, 0, len(collections))

	for _, col := range collections {
		if !matches(pattern, col.Namespace, col.Name) {
			continue
		}

		cols = append(cols, output.CollectionRow{
			Namespace: col.Namespace,
			Name:      col.Name,
			Arrows:    col.ArrowCount,
		})
	}

	return output.NewCatalog(rows, cols, pattern)
}

// installedRefAndState reads the arrow's first installed version. An arrow
// with no versions is in the catalog but not installed anywhere.
func installedRefAndState(arrow apidto.ArrowListItemDTO) (ref, state string) {
	if len(arrow.Versions) == 0 {
		return "-", "absent"
	}

	ref, state = arrow.Versions[0].Ref, arrow.Versions[0].State
	if ref == "" {
		ref = "-"
	}

	return ref, state
}

func viewCatalog(c output.Catalog, t theme.Theme) string {
	var b strings.Builder

	b.WriteString(t.Header.Render("ARROWS") + "\n")
	b.WriteString(component.Table(
		[]component.Column{
			{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "REF"}, {Title: "STATE"},
		},
		arrowCatalogRows(c.Arrows, t),
		"no arrows",
		t,
	))

	b.WriteString("\n" + t.Header.Render("COLLECTIONS") + "\n")
	b.WriteString(component.Table(
		[]component.Column{
			{Title: "NAMESPACE"}, {Title: "NAME"}, {Title: "ARROWS"},
		},
		collectionCatalogRows(c.Collections),
		"no collections",
		t,
	))

	// The count is shown only for a filtered listing, where it answers "how
	// much did the pattern keep". Unfiltered it just restates the table.
	if c.Query != "" {
		b.WriteString("\n" + t.Muted.Render(
			strconv.Itoa(c.Total)+" result(s) for "+c.Query,
		) + "\n")
	}

	return b.String()
}

func arrowCatalogRows(arrows []output.ArrowRow, t theme.Theme) [][]string {
	rows := make([][]string, 0, len(arrows))
	for _, arrow := range arrows {
		rows = append(rows, []string{
			arrow.Namespace, arrow.Name, arrow.Ref,
			t.State(domain.ArrowState(arrow.State)),
		})
	}

	return rows
}

func collectionCatalogRows(collections []output.CollectionRow) [][]string {
	rows := make([][]string, 0, len(collections))
	for _, col := range collections {
		rows = append(rows, []string{
			col.Namespace, col.Name, strconv.Itoa(col.Arrows),
		})
	}

	return rows
}
