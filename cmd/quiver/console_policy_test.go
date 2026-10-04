package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func walkCommands(
	cmd *cobra.Command,
	visit func(*cobra.Command),
) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(child, visit)
	}
}

func TestConsolePolicy_TheProcessCommandsAreNeverReachable(t *testing.T) {
	root := newRootCmd()

	for _, line := range []string{"daemon", "daemon --host tcp://0.0.0.0:1", "self-update /tmp/x", "completion bash", "help", "context list", "auth devices list"} {
		_, err := command.Authorize(root, strings.Fields(line))
		assert.Error(t, err, line)
	}
}

func TestConsolePolicy_OnlyAnnotatedChainsInTheRealRootAreReachable(t *testing.T) {
	root := newRootCmd()

	walkCommands(root, func(cmd *cobra.Command) {
		if cmd == root {
			assert.False(t, clierr.IsConsole(cmd))
			return
		}
		if !clierr.IsConsole(cmd) {
			return
		}
		for parent := cmd.Parent(); parent != nil && parent != root; parent = parent.Parent() {
			assert.True(t, clierr.IsConsole(parent), "%s is annotated but %s is not", cmd.CommandPath(), parent.CommandPath())
		}
		assert.False(t, cmd.Hidden, "%s is hidden yet annotated", cmd.CommandPath())
	})
}

func TestConsolePolicy_NamedProcessCommandsCarryNoAnnotation(t *testing.T) {
	walkCommands(newRootCmd(), func(cmd *cobra.Command) {
		switch cmd.Name() {
		case "daemon", "self-update", "context", "auth", "completion", "help":
			assert.False(t, clierr.IsConsole(cmd), cmd.CommandPath())
		}
	})
}
