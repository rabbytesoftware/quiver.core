package commands

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// bareNS strips the @ref: manifest endpoints address arrows by bare namespace.
func bareNS(ns string) string {
	return domain.Namespace(ns).BareNamespace().String()
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

func (a *app) listCmd() *cobra.Command {
	var filter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List catalog arrows and followed collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runCatalog(cmd, filter)
		},
	}
	cmd.Flags().StringVarP(&filter, "filter", "F", "", "glob or substring filter")
	return cmd
}

func (a *app) searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <pattern>",
		Short: "Search arrows and collections by glob or substring",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runCatalog(cmd, args[0])
		},
	}
}

// printManifest writes the compiled manifest verbatim. It is the one output
// that is not a payload: the bytes are the answer, and re-encoding them
// through an output format would corrupt what the caller asked to see.
func (a *app) printManifest(cmd *cobra.Command, ns string) error {
	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	var raw json.RawMessage

	if err := a.withSpinner(cmd, "loading", func() error {
		var e error
		raw, e = cli.GetArrowManifest(cmd.Context(), bareNS(ns))

		return e
	}); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(raw))

	return nil
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
			if manifest {
				return a.printManifest(cmd, args[0])
			}

			return runInstant(
				a, cmd, "loading "+args[0],
				func(cli *client.Client) (apidto.ArrowDetailDTO, error) {
					return cli.GetArrow(cmd.Context(), args[0])
				},
				viewArrowDetail,
			)
		},
	}
	cmd.Flags().BoolVar(&manifest, "manifest", false, "print the compiled manifest")
	return cmd
}

// methodInfo is one entry in the methods listing.
type methodInfo struct {
	Name        string   `json:"name" yaml:"name"`
	AvailableIn []string `json:"available_in,omitempty" yaml:"available_in,omitempty"`
	Builtin     bool     `json:"builtin,omitempty" yaml:"builtin,omitempty"`
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
			return runInstant(
				a, cmd, "loading methods",
				func(cli *client.Client) ([]methodInfo, error) {
					raw, err := cli.GetArrowManifest(cmd.Context(), bareNS(args[0]))
					if err != nil {
						return nil, err
					}

					methods, err := manifestMethods(raw)
					if err != nil {
						return nil, err
					}

					if includeBuiltins {
						methods = append(builtinMethods(), methods...)
					}

					return methods, nil
				},
				viewMethods,
			)
		},
	}
	cmd.Flags().BoolVar(&includeBuiltins, "include-builtins", false, "also list lifecycle methods")
	return cmd
}
