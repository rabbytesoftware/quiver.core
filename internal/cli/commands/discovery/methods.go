package discovery

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

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

func (c *commands) methodsCmd() *cobra.Command {
	var includeBuiltins bool
	cmd := &cobra.Command{
		Use:   "methods <namespace>",
		Short: "List an arrow's custom methods",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clierr.ValidNS(args[0]); err != nil {
				return err
			}
			return invoke.RunInstant(
				c.sess, c.rb, cmd, "loading methods",
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
