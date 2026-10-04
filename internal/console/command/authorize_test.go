package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/commands/clierr"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func executor() command.Executor {
	return command.New(command.Options{
		ServerURI: "http://127.0.0.1:1",
		Version:   "test",
	})
}

func prepare(
	t *testing.T,
	line string,
) error {
	t.Helper()

	tokens, err := command.Tokenize(line)
	require.NoError(t, err, line)
	_, err = executor().Prepare(tokens, "")
	return err
}

func TestExecutor_Prepare_AllowsTheAnnotatedCommands(t *testing.T) {
	for _, line := range []string{
		"install github.com/a/b",
		"run github.com/a/b",
		"stop github.com/a/b",
		"update github.com/a/b",
		"uninstall github.com/a/b --yes",
		"ps",
		"status",
		"status github.com/a/b",
		"list",
		"search crow",
		"info github.com/a/b",
		"methods github.com/a/b",
		"health",
		"version",
		"arrow add github.com/a/b",
		"arrow remove github.com/a/b --yes",
		"arrow refresh github.com/a/b",
		"arrow list",
		"arrow show github.com/a/b",
		"collection list",
		"collection show github.com/a/b",
		"install github.com/a/b --help",
		"-o json list",
		"list -o yaml",
	} {
		assert.NoError(t, prepare(t, line), line)
	}
}

func TestExecutor_Prepare_DeniesEverythingElse(t *testing.T) {
	for _, line := range []string{
		"daemon",
		"daemon --host tcp://0.0.0.0:1",
		"self-update /tmp/quiver",
		"context list",
		"context add x --server tcp://evil:1",
		"context use x",
		"auth devices list",
		"auth devices revoke abc",
		"auth",
		"path status",
		"path setup",
		"path",
		"watch github.com/a/b",
		"arrow seed github.com/a/b --file /etc/passwd",
		"arrow seed github.com/a/b --file -",
		"collection seed github.com/a/b --file /etc/passwd",
		"collection follow github.com/a/b",
		"collection unfollow github.com/a/b",
		"collection update github.com/a/b",
		"help",
		"help install",
		"--help",
		"-h",
		"completion bash",
		"completion zsh",
		"__complete install",
		"__completeNoDesc install",
		"github.com/user/repo",
		"github.com/user/repo run",
		"github.com/user/repo someMethod --data a=b",
		"nonsense",
		"arrow",
		"collection",
		"arrow nonsense",
		"INSTALL github.com/a/b",
		"Install github.com/a/b",
		"inst github.com/a/b",
		"ins github.com/a/b",
		"arr add github.com/a/b",
		"arrow ad github.com/a/b",
		"arrow rem github.com/a/b",
		"ps2",
		"-o json",
		"-o json github.com/a/b",
		"--detach github.com/a/b run",
	} {
		err := prepare(t, line)
		var denied *command.DeniedError
		assert.True(t, errors.As(err, &denied), "%q must be denied, got %v", line, err)
	}
}

func TestExecutor_Prepare_DeniesTheRedirectingFlagsInAnySpelling(t *testing.T) {
	for _, line := range []string{
		"list --server unix:///tmp/x.sock",
		"list --server=unix:///tmp/x.sock",
		"--server tcp://evil:1 list",
		"--server=tcp://evil:1 list",
		"install x --context other",
		"install x --context=other",
		"--context other health",
		"list --config /tmp/cfg.yaml",
		"list --config=/tmp/cfg.yaml",
		"--config=/tmp/cfg.yaml version",
		"list -- --server x",
	} {
		err := prepare(t, line)
		var denied *command.DeniedError
		assert.True(t, errors.As(err, &denied), "%q must be denied, got %v", line, err)
	}
}

func TestExecutor_Prepare_DeniesTheWatchFlagOnStatus(t *testing.T) {
	for _, line := range []string{
		"status github.com/a/b --watch",
		"status github.com/a/b -w",
		"status --watch github.com/a/b",
		"status -w",
		"status --watch=true github.com/a/b",
	} {
		err := prepare(t, line)
		var denied *command.DeniedError
		require.True(t, errors.As(err, &denied), "%q must be denied, got %v", line, err)
		assert.Contains(t, denied.Error(), "watch")
	}
}

func TestExecutor_Prepare_UnknownFlagsAreLeftToCobra(t *testing.T) {
	assert.NoError(t, prepare(t, "list --no-such-flag"))
}

func TestExecutor_Prepare_WithoutAServerAddressIsUnavailable(t *testing.T) {
	_, err := command.New(command.Options{}).Prepare([]string{"list"}, "")

	assert.ErrorIs(t, err, command.ErrUnavailable)
}

func TestAuthorize_DefaultDeny_NoCommandWithoutTheAnnotationIsReachable(t *testing.T) {
	root := command.NewRootForTest(executor())
	walk(root, func(cmd *cobra.Command) {
		if cmd == root || clierr.IsConsole(cmd) {
			return
		}
		_, err := command.Authorize(root, strings.Fields(strings.TrimPrefix(cmd.CommandPath(), "quiver ")))
		assert.Error(t, err, "%s must not be reachable", cmd.CommandPath())
	})
}

func TestAuthorize_DefaultDeny_EveryAnnotatedCommandHasAnnotatedAncestors(t *testing.T) {
	root := command.NewRootForTest(executor())
	walk(root, func(cmd *cobra.Command) {
		if cmd == root || !clierr.IsConsole(cmd) {
			return
		}
		for parent := cmd.Parent(); parent != nil && parent != root; parent = parent.Parent() {
			assert.True(t, clierr.IsConsole(parent), "%s is annotated but its parent %s is not", cmd.CommandPath(), parent.CommandPath())
		}
	})
}

func TestAuthorize_DefaultDeny_TheAnnotatedLeafCommandsAreExactlyTheApprovedSet(t *testing.T) {
	root := command.NewRootForTest(executor())
	var got []string
	walk(root, func(cmd *cobra.Command) {
		if cmd != root && cmd.Runnable() && clierr.IsConsole(cmd) {
			got = append(got, strings.TrimPrefix(cmd.CommandPath(), "quiver "))
		}
	})

	assert.ElementsMatch(t, []string{
		"install", "run", "stop", "update", "uninstall",
		"ps", "status", "list", "search", "info", "methods", "health", "version",
		"arrow add", "arrow remove", "arrow refresh", "arrow list", "arrow show",
		"collection list", "collection show",
	}, got)
}

func TestAuthorize_DefaultDeny_NamedDangerousCommandsAreNeverAnnotated(t *testing.T) {
	root := command.NewRootForTest(executor())
	forbidden := []string{"daemon", "self-update", "context", "auth", "completion", "help", "path", "watch", "seed", "follow", "unfollow"}
	walk(root, func(cmd *cobra.Command) {
		for _, name := range forbidden {
			assert.False(t, cmd.Name() == name && clierr.IsConsole(cmd), "%s must never carry the console annotation", cmd.CommandPath())
		}
	})
}

func TestAuthorize_DefaultDeny_NoHiddenCommandIsAnnotated(t *testing.T) {
	root := command.NewRootForTest(executor())
	walk(root, func(cmd *cobra.Command) {
		assert.False(t, cmd.Hidden && clierr.IsConsole(cmd), cmd.CommandPath())
	})
}

func TestAuthorize_DefaultDeny_TheRootItselfIsNeverAnnotated(t *testing.T) {
	assert.False(t, clierr.IsConsole(command.NewRootForTest(executor())))
}

func TestExecutor_Commands_ListsOnlyReachableCommandsWithoutDeniedFlags(t *testing.T) {
	infos := executor().Commands()

	paths := make([]string, 0, len(infos))
	for _, info := range infos {
		paths = append(paths, strings.Join(info.Path, " "))
		assert.NotEmpty(t, info.Usage)
		assert.NotNil(t, info.Flags)
		assert.NotNil(t, info.Aliases)
	}
	assert.Contains(t, paths, "arrow add")
	assert.Contains(t, paths, "install")
	for _, hidden := range []string{"daemon", "context list", "arrow seed", "path status", "watch", "completion"} {
		assert.NotContains(t, paths, hidden)
	}
	for _, info := range infos {
		if strings.Join(info.Path, " ") != "status" {
			continue
		}
		for _, flag := range info.Flags {
			assert.NotEqual(t, "watch", flag.Name)
		}
	}
}

func walk(
	cmd *cobra.Command,
	visit func(*cobra.Command),
) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walk(child, visit)
	}
}

func TestAuthorize_DefaultDeny_NoCommandShadowsTheRootGuardWithItsOwnHooks(t *testing.T) {
	root := command.NewRootForTest(executor())
	walk(root, func(cmd *cobra.Command) {
		if cmd == root {
			return
		}
		assert.Nil(t, cmd.PersistentPreRun, cmd.CommandPath())
		assert.Nil(t, cmd.PersistentPreRunE, cmd.CommandPath())
	})
}

func TestAuthorize_DefaultDeny_CobraPrefixAndCaseMatchingStayOff(t *testing.T) {
	assert.False(t, cobra.EnablePrefixMatching)
	assert.False(t, cobra.EnableCaseInsensitive)
	assert.False(t, cobra.EnableTraverseRunHooks)
}
