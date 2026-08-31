package commands

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// dispatch is the root RunE: `quiver <namespace> [method]` routes custom
// manifest methods; a bare namespace shows what can be done with it.
func (a *app) dispatch(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	ns := args[0]
	if domain.Namespace(ns).Validate() != nil {
		return usageErrorf("unknown command %q — run 'quiver --help'", ns)
	}
	if len(args) == 1 {
		return a.namespacePanel(cmd, ns)
	}

	detach, _ := cmd.Flags().GetBool("detach")
	data, _ := cmd.Flags().GetStringArray("data")
	return a.runMethod(cmd, ns, args[1], methodOpts{detach: detach, data: data})
}

// Panel is the payload of a bare `quiver <namespace>`: what can be done with
// that arrow. It is a payload like any other so the panel is scriptable —
// `quiver <ns> -o json | jq .lifecycle` answers "what can I run".
type Panel struct {
	Subject   string   `json:"subject" yaml:"subject"`
	Lifecycle []string `json:"lifecycle" yaml:"lifecycle"`
	Methods   []string `json:"methods" yaml:"methods"`
	Discovery []string `json:"discovery" yaml:"discovery"`
}

// namespacePanel shows what quiver can do with a namespace.
//
// Custom methods come from the manifest, so the panel needs the daemon. A
// daemon that cannot answer still yields a useful panel: the lifecycle verbs
// are the same for every arrow, so the methods list is simply omitted rather
// than failing the command.
func (a *app) namespacePanel(cmd *cobra.Command, ns string) error {
	return runInstant(
		a, cmd, "loading methods",
		func(cli *client.Client) (Panel, error) {
			panel := Panel{
				Subject:   ns,
				Lifecycle: []string{"install", "run", "stop", "update", "uninstall"},
				Discovery: []string{"info", "methods", "arrow refresh"},
			}

			raw, err := cli.GetArrowManifest(cmd.Context(), bareNS(ns))
			if err != nil {
				return panel, nil
			}

			methods, err := manifestMethods(raw)
			if err != nil {
				return panel, nil
			}

			panel.Methods = make([]string, 0, len(methods))
			for _, m := range methods {
				panel.Methods = append(panel.Methods, m.Name)
			}

			return panel, nil
		},
		viewPanel,
	)
}

func viewPanel(p Panel, t theme.Theme) string {
	var b strings.Builder

	b.WriteString(t.Header.Render(p.Subject) + "\n\n")

	b.WriteString(t.Label.Render("Lifecycle") + "\n")
	for _, op := range p.Lifecycle {
		b.WriteString("  quiver " + op + " " + p.Subject + "\n")
	}

	if len(p.Methods) > 0 {
		b.WriteString("\n" + t.Label.Render("Methods") + "\n")
		for _, m := range p.Methods {
			b.WriteString("  " + m + "\n")
		}
	}

	b.WriteString("\n" + t.Label.Render("Discovery") + "\n")
	for _, op := range p.Discovery {
		b.WriteString("  quiver " + op + " " + p.Subject + "\n")
	}

	b.WriteString("\n" + t.Muted.Render(
		"custom methods run as: quiver "+p.Subject+" <method>",
	) + "\n")

	return b.String()
}
