package console

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
)

func TestParseLine_PlainWords_SplitsOnWhitespace(
	t *testing.T,
) {
	words, err := parseLine("install  github.com/x/y@v1 --yes")

	assert.NoError(t, err)
	assert.Equal(t, []string{"install", "github.com/x/y@v1", "--yes"}, words)
}

func TestParseLine_LengthBoundary_AcceptsUpTo1024Bytes(
	t *testing.T,
) {
	exact := strings.Repeat("a", 1024)

	words, err := parseLine(exact)
	_, tooLongErr := parseLine(exact + "a")

	assert.NoError(t, err)
	assert.Equal(t, []string{exact}, words)
	assert.ErrorIs(t, tooLongErr, errBadLine)
}

func TestParseLine_RejectedLines_ReturnTheLineError(
	t *testing.T,
) {
	lines := []string{"", "   ", "ps\x00", "ps\nps", "ps\tps", "ps\rps", "ps\x7f", "ps\x1b[31m", "ps\u0085ps", "ps\u009fps"}
	for _, char := range shellSyntax {
		lines = append(lines, "ps a"+string(char)+"b")
	}

	for _, line := range lines {
		_, err := parseLine(line)

		assert.ErrorIs(t, err, errBadLine, "%q", line)
	}
}

func markedTree() *cobra.Command {
	root := &cobra.Command{Use: "quiver"}
	leaf := &cobra.Command{Use: "leaf", Run: noop}
	open := &cobra.Command{Use: "open", Run: noop}
	group := &cobra.Command{Use: "group"}
	group.AddCommand(leaf, &cobra.Command{Use: "stray", Run: noop})
	root.AddCommand(group, open)
	clierr.AllowInConsole(leaf, open)
	return root
}

func noop(
	_ *cobra.Command,
	_ []string,
) {
}

func assertDenied(
	t *testing.T,
	root *cobra.Command,
	line string,
	wantName string,
) {
	t.Helper()

	_, err := allowed(root, strings.Fields(line))

	assert.ErrorContains(t, err, "command \""+wantName+"\" is not available", line)
}

func assertResolves(
	t *testing.T,
	root *cobra.Command,
	line string,
	wantPath string,
) {
	t.Helper()

	cmd, err := allowed(root, strings.Fields(line))

	assert.NoError(t, err, line)
	assert.Equal(t, wantPath, cmd.CommandPath(), line)
}

func TestAllowed_MarkedTree_NeedsTheMarkOnTheCommandAndEveryAncestor(
	t *testing.T,
) {
	assertResolves(t, markedTree(), "open", "quiver open")
	assertResolves(t, markedTree(), "open --extra", "quiver open")
	assertDenied(t, markedTree(), "group leaf", "group leaf")
	assertDenied(t, markedTree(), "group stray", "group stray")
	assertDenied(t, markedTree(), "nope now", "nope")
	assertDenied(t, markedTree(), "--help", "--help")
}

func TestAllowed_RealTree_FlagsBeforeTheCommandNeverReachAnUnmarkedOne(
	t *testing.T,
) {
	root := New(nil, "v").tree(nil)

	assertResolves(t, root, "-o json arrow list", "quiver arrow list")
	assertResolves(t, root, "--server tcp://x arrow list", "quiver arrow list")
	assertResolves(t, root, "arrow list --server x", "quiver arrow list")
	assertResolves(t, root, "--output json ps", "quiver ps")
	assertResolves(t, root, "--help arrow list", "quiver list")
	assertResolves(t, root, "list --help", "quiver list")
	assertResolves(t, root, "arrow -- seed x", "quiver arrow")
	assertDenied(t, root, "arrow --output json seed x", "arrow seed")
	assertDenied(t, root, "--server x daemon", "--server")
	assertDenied(t, root, "-o json", "-o")
	assertDenied(t, root, "-- daemon", "--")
}

func TestAllowed_RealTree_ExposesExactlyTheApprovedCommands(
	t *testing.T,
) {
	root := New(nil, "v").tree(nil)

	assert.ElementsMatch(t, []string{
		"arrow", "arrow add", "arrow list", "arrow refresh", "arrow remove", "arrow show",
		"collection", "collection list", "collection show",
		"health", "version", "info", "list", "methods", "search",
		"install", "run", "stop", "uninstall", "update", "ps", "status",
	}, reachable(root, root, nil), "a new command must be opted in on purpose")
}

func reachable(
	root *cobra.Command,
	parent *cobra.Command,
	found []string,
) []string {
	for _, child := range parent.Commands() {
		found = appendIfReachable(root, child, found)
	}
	return found
}

func appendIfReachable(
	root *cobra.Command,
	cmd *cobra.Command,
	found []string,
) []string {
	path := strings.TrimPrefix(cmd.CommandPath(), "quiver ")
	if _, err := allowed(root, strings.Fields(path)); err == nil {
		found = append(found, path)
	}
	return reachable(root, cmd, found)
}

func TestAllowed_RealTree_NeverReachesTheProcessCommands(
	t *testing.T,
) {
	root := New(nil, "v").tree(nil)

	for _, line := range []string{
		"daemon", "self-update x", "context list", "auth devices list", "completion bash", "help",
		"__complete a", "watch", "arrow seed x", "collection follow x", "path", "github.com/x/y install",
	} {
		_, err := allowed(root, strings.Fields(line))

		assert.Error(t, err, line)
	}
}
