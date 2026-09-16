package discovery

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/collection"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func (c *commands) listCmd() *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows and followed collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return c.runCatalog(cmd, filter)
		},
	}
	cmd.Flags().StringVarP(&filter, "filter", "F", "", "glob or substring filter")
	return cmd
}

func (c *commands) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <pattern>",
		Short: "Search arrows and collections by glob or substring",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runCatalog(cmd, args[0])
		},
	}
}

// runCatalog fetches the catalog, filters it by pattern, and renders it.
//
// list and search differ only in how the pattern arrives — a --filter flag
// versus a positional argument — so they share everything below. The payload
// carries Query, which is what tells the two apart after the fact.
func (c *commands) runCatalog(cmd *cobra.Command, pattern string) error {
	return invoke.RunInstant(
		c.sess, c.rb, cmd, "loading catalog",
		func(cli *client.Client) (output.Catalog, error) {
			arrows, err := cli.ListArrows(cmd.Context(), nil)
			if err != nil {
				return output.Catalog{}, err
			}
			collections, err := cli.ListCollections(cmd.Context())
			if err != nil {
				return output.Catalog{}, err
			}
			return buildCatalog(arrows, collections, pattern), nil
		},
		viewCatalog,
	)
}

// buildCatalog filters the two listings by pattern and shapes them into the
// payload. An empty pattern matches everything.
func buildCatalog(
	arrows []apidto.ArrowListItemDTO,
	collections []apidto.CollectionListItemDTO,
	pattern string,
) output.Catalog {
	rows := make([]output.ArrowRow, 0, len(arrows))
	for _, a := range arrows {
		if !matches(pattern, a.Namespace, a.Name) {
			continue
		}
		ref, state := arrow.InstalledRefAndState(a)
		rows = append(rows, output.ArrowRow{Namespace: a.Namespace, Name: a.Name, Ref: ref, State: state})
	}

	cols := make([]output.CollectionRow, 0, len(collections))
	for _, col := range collections {
		if !matches(pattern, col.Namespace, col.Name) {
			continue
		}
		cols = append(cols, output.CollectionRow{Namespace: col.Namespace, Name: col.Name, Arrows: col.ArrowCount})
	}

	return output.NewCatalog(rows, cols, pattern)
}

func viewCatalog(c output.Catalog, t theme.Theme) string {
	var b strings.Builder
	b.WriteString(t.Header.Render("ARROWS") + "\n")
	b.WriteString(arrow.Table(c.Arrows, t))
	b.WriteString("\n" + t.Header.Render("COLLECTIONS") + "\n")
	b.WriteString(collection.Table(c.Collections, t))
	// The count is shown only for a filtered listing, where it answers "how
	// much did the pattern keep". Unfiltered it just restates the table.
	if c.Query != "" {
		b.WriteString("\n" + t.Muted.Render(strconv.Itoa(c.Total)+" result(s) for "+c.Query) + "\n")
	}
	return b.String()
}

// matches reports whether a namespace or name satisfies a glob or substring
// pattern. Empty patterns match everything. Glob wildcards (* and ?) cross
// path separators, so "*" matches every namespace and "*repo" matches a full
// domain/user/repo namespace.
func matches(pattern, ns, name string) bool {
	if pattern == "" {
		return true
	}
	if globMatch(pattern, ns) || globMatch(pattern, name) {
		return true
	}
	return strings.Contains(
		strings.ToLower(ns+" "+name),
		strings.ToLower(pattern),
	)
}

// globMatch reports whether s matches a shell-style glob (case-insensitive),
// where * matches any run of characters (including /) and ? matches any single
// character. A pattern with no wildcards must match s in full.
func globMatch(pattern, s string) bool {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
