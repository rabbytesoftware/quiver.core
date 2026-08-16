package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// bareNS strips the @ref: manifest endpoints address arrows by bare namespace.
func bareNS(ns string) string {
	return domain.Namespace(ns).BareNamespace().String()
}

// catalogDoc is the combined list/search payload.
type catalogDoc struct {
	Arrows      []apidto.ArrowListItemDTO      `json:"arrows"`
	Collections []apidto.CollectionListItemDTO `json:"collections"`
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

// fetchCatalog loads arrows and collections, filtered by pattern.
func (a *app) fetchCatalog(cmd *cobra.Command, pattern string) (catalogDoc, error) {
	cli, err := a.session(cmd)
	if err != nil {
		return catalogDoc{}, err
	}
	var (
		arrows      []apidto.ArrowListItemDTO
		collections []apidto.CollectionListItemDTO
	)
	if err := a.withSpinner(cmd, "loading", func() error {
		var e error
		arrows, e = cli.ListArrows(cmd.Context(), nil)
		if e != nil {
			return e
		}
		collections, e = cli.ListCollections(cmd.Context())
		return e
	}); err != nil {
		return catalogDoc{}, err
	}

	doc := catalogDoc{
		Arrows:      []apidto.ArrowListItemDTO{},
		Collections: []apidto.CollectionListItemDTO{},
	}
	for _, arrow := range arrows {
		if matches(pattern, arrow.Namespace, arrow.Name) {
			doc.Arrows = append(doc.Arrows, arrow)
		}
	}
	for _, col := range collections {
		if matches(pattern, col.Namespace, col.Name) {
			doc.Collections = append(doc.Collections, col)
		}
	}
	return doc, nil
}

// arrowTableHeaders match arrowRows. REF is the ref the arrow was registered
// under — the handle arrow remove expects.
func arrowTableHeaders() []string {
	return []string{"NAMESPACE", "NAME", "REF", "STATE"}
}

func arrowRows(arrows []apidto.ArrowListItemDTO) [][]string {
	rows := make([][]string, 0, len(arrows))
	for _, arrow := range arrows {
		ref, state := "-", "absent"
		if len(arrow.Versions) > 0 {
			if arrow.Versions[0].Ref != "" {
				ref = arrow.Versions[0].Ref
			}
			state = arrow.Versions[0].State
		}
		rows = append(rows, []string{
			arrow.Namespace, arrow.Name, ref, ui.StateLabel(state),
		})
	}
	return rows
}

func collectionRows(collections []apidto.CollectionListItemDTO) [][]string {
	rows := make([][]string, 0, len(collections))
	for _, col := range collections {
		rows = append(rows, []string{
			col.Namespace, col.Name, fmt.Sprintf("%d", col.ArrowCount),
		})
	}
	return rows
}

func writeCatalogTables(w io.Writer, doc catalogDoc) {
	_, _ = fmt.Fprintln(w, "  "+ui.Bold.Render("ARROWS"))
	_, _ = fmt.Fprint(w, ui.RenderTable(arrowTableHeaders(), arrowRows(doc.Arrows)))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  "+ui.Bold.Render("COLLECTIONS"))
	_, _ = fmt.Fprint(w, ui.RenderTable(
		[]string{"NAMESPACE", "NAME", "ARROWS"},
		collectionRows(doc.Collections),
	))
}

func (a *app) listCmd() *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows and followed collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.fetchCatalog(cmd, filter)
			if err != nil {
				return err
			}
			return a.render(cmd, doc, func(w io.Writer) error {
				_, _ = fmt.Fprint(w, ui.CommandHeader("list", ""))
				_, _ = fmt.Fprintln(w)
				writeCatalogTables(w, doc)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&filter, "filter", "F", "", "glob or substring filter")
	return cmd
}

// searchDoc adds the match count to the catalog payload.
type searchDoc struct {
	Count int `json:"count"`
	catalogDoc
}

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <pattern>",
		Short: "Search arrows and collections by glob or substring",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			doc, err := a.fetchCatalog(cmd, args[0])
			if err != nil {
				return err
			}
			result := searchDoc{
				Count:      len(doc.Arrows) + len(doc.Collections),
				catalogDoc: doc,
			}
			return a.render(cmd, result, func(w io.Writer) error {
				_, _ = fmt.Fprint(w, ui.CommandHeader("search", args[0]))
				_, _ = fmt.Fprintln(w)
				writeCatalogTables(w, doc)
				_, _ = fmt.Fprintln(w)
				_, _ = fmt.Fprintf(w, "  %d result(s)\n", result.Count)
				return nil
			})
		},
	}
}

func (a *app) infoCmd() *cobra.Command {
	var manifest bool
	cmd := &cobra.Command{
		Use:   "info <namespace>",
		Short: "Show an arrow's detail (or raw manifest)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			if manifest {
				var raw json.RawMessage
				if err := a.withSpinner(cmd, "loading", func() error {
					var e error
					raw, e = cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))
					return e
				}); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				return nil
			}
			var detail apidto.ArrowDetailDTO
			if err := a.withSpinner(cmd, "loading", func() error {
				var e error
				detail, e = cli.GetArrow(cmd.Context(), args[0])
				return e
			}); err != nil {
				return err
			}
			return a.render(cmd, detail, func(w io.Writer) error {
				writeArrowDetail(w, detail)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&manifest, "manifest", false, "print the compiled manifest")
	return cmd
}

func writeArrowDetail(w io.Writer, d apidto.ArrowDetailDTO) {
	_, _ = fmt.Fprint(w, ui.CommandHeader("info", d.Namespace))
	_, _ = fmt.Fprintln(w)
	kv := func(k, v string) {
		if v != "" {
			_, _ = fmt.Fprintf(w, "  %s  %s\n", ui.Muted.Render(fmt.Sprintf("%-12s", k)), v)
		}
	}
	kv("Name", d.Name)
	kv("State", ui.StateLabel(d.State))
	kv("Description", d.Description)
	kv("License", d.License)
	kv("Tags", strings.Join(d.Tags, ", "))
	kv("Constraint", d.InstalledConstraint)
	kv("Installed", d.InstalledAt)
}

// methodInfo is one entry in the methods listing.
type methodInfo struct {
	Name        string   `json:"name"`
	AvailableIn []string `json:"available_in,omitempty"`
	Builtin     bool     `json:"builtin,omitempty"`
}

// manifestMethods extracts custom method names from a compiled manifest.
func manifestMethods(raw json.RawMessage) ([]methodInfo, error) {
	var doc struct {
		Targets map[string]struct {
			Methods map[string]struct {
				AvailableIn []string `json:"available_in"`
			} `json:"methods"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	byName := map[string]methodInfo{}
	for _, target := range doc.Targets {
		for name, m := range target.Methods {
			byName[name] = methodInfo{Name: name, AvailableIn: m.AvailableIn}
		}
	}
	methods := make([]methodInfo, 0, len(byName))
	for _, m := range byName {
		methods = append(methods, m)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })
	return methods, nil
}

func builtinMethods() []methodInfo {
	names := []string{"install", "run", "stop", "update", "uninstall"}
	methods := make([]methodInfo, 0, len(names))
	for _, name := range names {
		methods = append(methods, methodInfo{Name: name, Builtin: true})
	}
	return methods
}

func (a *app) methodsCmd() *cobra.Command {
	var includeBuiltins bool
	cmd := &cobra.Command{
		Use:   "methods <namespace>",
		Short: "List an arrow's custom methods",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}
			cli, err := a.session(cmd)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := a.withSpinner(cmd, "loading", func() error {
				var e error
				raw, e = cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))
				return e
			}); err != nil {
				return err
			}
			methods, err := manifestMethods(raw)
			if err != nil {
				return err
			}
			if includeBuiltins {
				methods = append(builtinMethods(), methods...)
			}
			return a.render(cmd, methods, func(w io.Writer) error {
				_, _ = fmt.Fprint(w, ui.CommandHeader("methods", args[0]))
				_, _ = fmt.Fprintln(w)
				rows := make([][]string, 0, len(methods))
				for _, m := range methods {
					kind := "custom"
					if m.Builtin {
						kind = "built-in"
					}
					rows = append(rows, []string{
						m.Name, kind, strings.Join(m.AvailableIn, ", "),
					})
				}
				_, _ = fmt.Fprint(w, ui.RenderTable([]string{"METHOD", "KIND", "AVAILABLE IN"}, rows))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&includeBuiltins, "include-builtins", false, "also list lifecycle methods")
	return cmd
}
