package console

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

func TestParseLine_SplitsOnSpacesOnly(t *testing.T) {
	got, err := parseLine("install  github.com/x/y@v1 --yes")

	assert.NoError(t, err)
	assert.Equal(t, []string{"install", "github.com/x/y@v1", "--yes"}, got)
}

func TestParseLine_RejectsEmptyOversizedControlAndShellSyntax(t *testing.T) {
	lines := []string{"", "   ", strings.Repeat("a", 1025), "ps\x00", "ps\nps", "ps\tps", "ps\x7f"}
	for _, c := range shellSyntax {
		lines = append(lines, "ps a"+string(c)+"b")
	}
	for _, line := range lines {
		_, err := parseLine(line)
		assert.ErrorIs(t, err, errBadLine, "%q", line)
	}
}

func fakeTree() *cobra.Command {
	root := &cobra.Command{Use: "quiver"}
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}
	open := &cobra.Command{Use: "open", Run: func(*cobra.Command, []string) {}}
	group := &cobra.Command{Use: "group"}
	group.AddCommand(leaf, &cobra.Command{Use: "stray", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group, open)
	clierr.AllowInConsole(leaf, open)
	return root
}

func TestAllowed_NeedsTheMarkOnTheCommandAndEveryAncestor(t *testing.T) {
	testCases := map[string]string{
		"open":         "",
		"group leaf":   `"group leaf"`,
		"group stray":  `"group stray"`,
		"nope now":     `"nope"`,
		"--help":       `"--help"`,
		"open --extra": "",
	}
	for line, want := range testCases {
		_, err := allowed(fakeTree(), strings.Fields(line))

		if want == "" {
			assert.NoError(t, err, line)
		} else {
			assert.ErrorContains(t, err, want, line)
		}
	}
}

func TestAllowed_TheRealTreeExposesExactlyTheApprovedCommands(t *testing.T) {
	root := New(nil, "v").root(nil)
	var reachable []string
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			path := strings.TrimPrefix(child.CommandPath(), "quiver ")
			if _, err := allowed(root, strings.Fields(path)); err == nil {
				reachable = append(reachable, path)
			}
			walk(child)
		}
	}
	walk(root)

	assert.ElementsMatch(t, []string{
		"arrow", "arrow add", "arrow list", "arrow refresh", "arrow remove", "arrow show",
		"collection", "collection list", "collection show",
		"health", "version", "info", "list", "methods", "search",
		"install", "run", "stop", "uninstall", "update", "ps", "status",
	}, reachable, "a new command must be opted in on purpose")
}

func TestAllowed_TheRealTreeNeverReachesTheProcessCommands(t *testing.T) {
	for _, line := range []string{
		"daemon", "self-update x", "context list", "auth devices list", "completion bash", "help",
		"__complete a", "watch", "arrow seed x", "collection follow x", "path", "github.com/x/y install",
	} {
		_, err := allowed(New(nil, "v").root(nil), strings.Fields(line))
		assert.Error(t, err, line)
	}
}
