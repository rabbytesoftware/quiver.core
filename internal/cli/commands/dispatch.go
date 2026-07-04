package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
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

// namespacePanel prints what quiver can do with a namespace.
func (a *app) namespacePanel(cmd *cobra.Command, ns string) error {
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprint(w, ui.CommandHeader("", ns))
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  Lifecycle:")
	for _, op := range []string{"install", "run", "stop", "update", "uninstall"} {
		_, _ = fmt.Fprintf(w, "    quiver %-10s %s\n", op, ns)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "  Discovery:")
	_, _ = fmt.Fprintf(w, "    quiver info     %s\n", ns)
	_, _ = fmt.Fprintf(w, "    quiver methods  %s\n", ns)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "  Custom methods run as: quiver %s <method>\n", ns)
	return nil
}
