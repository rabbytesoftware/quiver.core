package commands

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/invoke"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// dispatch is the root RunE: `quiver <namespace> [method]` routes custom
// manifest methods; a bare namespace shows what can be done with it.
func (t *commandTree) dispatch(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	ns := args[0]
	if err := clierr.ValidNS(ns); err != nil {
		return err
	}
	if len(args) == 1 {
		return t.namespacePanel(cmd, ns)
	}

	detach, _ := cmd.Flags().GetBool("detach")
	data, _ := cmd.Flags().GetStringArray("data")

	// A bare `quiver <namespace> <method>` has no cobra command of its own
	// to run a manifest-defined method through — it shares the runtime
	// package's install/run/stop streaming machinery instead of duplicating
	// it here.
	return runtime.RunMethod(t.sess, t.rb, cmd, ns, args[1], detach, data)
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
// Custom methods come from the manifest, so the panel needs the daemon. Any
// failure to reach or read the manifest — including being unable to start
// the daemon, load config, or resolve the context — yields the lifecycle
// panel rather than an error.
func (t *commandTree) namespacePanel(cmd *cobra.Command, ns string) error {
	return invoke.RenderInstant(
		t.sess, t.rb, cmd, "loading methods",
		func() (Panel, error) {
			panel := Panel{
				Subject:   ns,
				Lifecycle: []string{"install", "run", "stop", "update", "uninstall"},
				Discovery: []string{"info", "methods", "arrow refresh"},
			}

			cli, err := t.sess.Client(cmd.Context(), cmd)
			if err != nil {
				return panel, nil
			}

			raw, err := cli.GetArrowManifest(cmd.Context(), domain.Namespace(ns).BareNamespace().String())
			if err != nil {
				return panel, nil
			}

			methods, err := namespaceMethods(raw)
			if err != nil {
				return panel, nil
			}

			panel.Methods = methods

			return panel, nil
		},
		viewPanel,
	)
}

// namespaceMethods extracts a manifest's custom method names, sorted.
//
// This duplicates, in miniature, discovery's manifestMethods/methodInfo
// (which also tracks AvailableIn and Builtin, for the `quiver methods`
// command): namespacePanel only ever needs the names, and exporting
// discovery's internal methodInfo type just to serve this one caller would
// grow discovery's public surface for no other benefit — the same
// "not worth exporting" reasoning this file's inlined bareNS check
// (domain.Namespace(ns).BareNamespace().String()) already applies to
// avoid a commands -> discovery dependency.
func namespaceMethods(raw json.RawMessage) ([]string, error) {
	var doc struct {
		Targets map[string]struct {
			Methods map[string]struct{} `json:"methods"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	names := map[string]struct{}{}
	for _, target := range doc.Targets {
		for name := range target.Methods {
			names[name] = struct{}{}
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)

	return out, nil
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
