package console

import (
	"bufio"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/session"
)

type testTree struct {
	started chan struct{}
}

func (tt *testTree) build(
	_ session.Session,
) *cobra.Command {
	root := &cobra.Command{Use: "quiver", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(clierr.AllowInConsole(
		&cobra.Command{Use: "echo", RunE: tt.echo},
		&cobra.Command{Use: "fail", RunE: tt.fail},
		&cobra.Command{Use: "panic", RunE: tt.panics},
		&cobra.Command{Use: "flood", RunE: tt.flood},
		&cobra.Command{Use: "wait", RunE: tt.wait},
		&cobra.Command{Use: "ask", RunE: tt.ask},
	)...)
	root.AddCommand(&cobra.Command{Use: "hidden", Run: noop})
	return root
}

func (tt *testTree) echo(
	cmd *cobra.Command,
	_ []string,
) error {
	fmt.Fprint(cmd.OutOrStdout(), "hello\n")
	fmt.Fprint(cmd.ErrOrStderr(), "careful\n")
	return nil
}

func (tt *testTree) fail(
	_ *cobra.Command,
	_ []string,
) error {
	return errors.New("boom")
}

func (tt *testTree) panics(
	_ *cobra.Command,
	_ []string,
) error {
	panic("kaboom")
}

func (tt *testTree) flood(
	cmd *cobra.Command,
	_ []string,
) error {
	for _, size := range []int{outputLimit - 1, 2, 7} {
		_, _ = cmd.OutOrStdout().Write([]byte(strings.Repeat("x", size)))
	}
	return nil
}

func (tt *testTree) wait(
	cmd *cobra.Command,
	_ []string,
) error {
	close(tt.started)
	<-cmd.Context().Done()
	return cmd.Context().Err()
}

func (tt *testTree) ask(
	cmd *cobra.Command,
	_ []string,
) error {
	_, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	return err
}
