//go:build windows

package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

func runShellWrapped(
	t *testing.T,
	command string,
) Process {
	t.Helper()

	config := models.NewConfig([]string{command})
	config.ShellWrap = true

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess(%s) error = %v", command, err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := proc.Wait(ctx); err != nil {
		t.Fatalf("Wait(%s) error = %v", command, err)
	}

	return proc
}

// The whole point of the fix, run for real: a quoted path that contains a
// space has to survive into cmd's own parser. Before CmdLine was set, go
// escaped the quotes as \" and the backslashes landed inside the path, so the
// installed case answered "not installed" and no error was raised anywhere.
func TestWindowsProcess_ShellWrap_QuotedPathWithSpacesIsFound(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Program Files", "Quiver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	installed := filepath.Join(dir, "quiverdesktop.exe")
	if err := os.WriteFile(installed, []byte("stub"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	testCases := []struct {
		name     string
		path     string
		wantExit int
	}{
		{name: "installed", path: installed, wantExit: 0},
		{name: "not installed", path: filepath.Join(dir, "missing.exe"), wantExit: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			command := `if exist "` + tc.path + `" (exit 0) else (exit 1)`

			if got := runShellWrapped(t, command).ExitCode(); got != tc.wantExit {
				t.Errorf("ExitCode = %d, want %d for %s", got, tc.wantExit, command)
			}
		})
	}
}

// A command that itself OPENS with a quote is the case a bare paste-in would
// still have broken: with no wrapper quotes added, cmd's own second quoting
// rule sees the command's leading quote as the line's first character and
// strips it together with the line's last quote, corrupting the command's own
// quoting rather than removing a pair that belongs to quiver. Bracketing adds
// its own outer pair first, so the character cmd's rule 2 finds at that
// position is quiver's, not the command's: exactly what cmdline_test.go's
// "quoted path with arguments" case pins at the string level. This test runs
// the same shape through a real cmd.exe, by invoking a copy of cmd.exe itself
// from a quoted, space-containing path with a further-quoted argument.
func TestWindowsProcess_ShellWrap_LeadingQuotedCommandIsFound(t *testing.T) {
	systemCmd := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	data, err := os.ReadFile(systemCmd)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", systemCmd, err)
	}

	dir := filepath.Join(t.TempDir(), "Program Files", "Quiver")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	nested := filepath.Join(dir, "nested.exe")
	if err := os.WriteFile(nested, data, 0o755); err != nil { // #nosec -- a real, runnable cmd.exe copy is the point of this test
		t.Fatalf("WriteFile() error = %v", err)
	}

	// The command handed to runShellWrapped is itself the "` + <quoted path> +
	// `" /C echo "..." shape cmdline_test.go already pins at the string level:
	// it opens with a quote (the copied cmd.exe's own quoted path) and carries
	// a second, later quoted segment (echo's argument).
	command := `"` + nested + `" /C echo found-the-nested-invocation`
	proc := runShellWrapped(t, command)

	if got := proc.ExitCode(); got != 0 {
		t.Errorf("ExitCode = %d, want 0 for %s", got, command)
	}
	if got := proc.Output(); !strings.Contains(got, "found-the-nested-invocation") {
		t.Errorf("Output = %q, want it to contain %q", got, "found-the-nested-invocation")
	}
}

// The simpler shapes that already worked have to keep working: setting CmdLine
// changes the bytes cmd receives for every run step, not only quoted ones.
func TestWindowsProcess_ShellWrap_UnquotedCommandsStillRun(t *testing.T) {
	testCases := []struct {
		name     string
		command  string
		wantOut  string
		wantExit int
	}{
		{name: "simple echo", command: "echo hello", wantOut: "hello", wantExit: 0},
		{name: "chained commands", command: "echo one && echo two", wantOut: "two", wantExit: 0},
		{name: "bare word", command: "ver", wantOut: "", wantExit: 0},
		{name: "unquoted path", command: `dir C:\Windows\System32\cmd.exe`, wantOut: "cmd.exe", wantExit: 0},
		{name: "non zero exit", command: "exit 3", wantOut: "", wantExit: 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proc := runShellWrapped(t, tc.command)

			if got := proc.ExitCode(); got != tc.wantExit {
				t.Errorf("ExitCode = %d, want %d for %s", got, tc.wantExit, tc.command)
			}
			if tc.wantOut != "" && !strings.Contains(proc.Output(), tc.wantOut) {
				t.Errorf("Output = %q, want it to contain %q", proc.Output(), tc.wantOut)
			}
		})
	}
}

// The wiring, not just the string: a shell-wrapped step must carry CmdLine, or
// syscall.StartProcess falls back to makeCmdLine and escapes the command again.
func TestWindowsProcess_ShellWrap_SetsCmdLine(t *testing.T) {
	config := models.NewConfig([]string{desktopProbe})
	config.ShellWrap = true

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := proc.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	wp, ok := proc.(*windowsProcess)
	if !ok {
		t.Fatalf("newProcess() returned %T, want *windowsProcess", proc)
	}
	if wp.cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil, so go's own escaping built the command line")
	}
	if got, want := wp.cmd.SysProcAttr.CmdLine, windowsShellCommandLine([]string{desktopProbe}); got != want {
		t.Errorf("CmdLine =\n\t%s\nwant\n\t%s", got, want)
	}
}

// A step that is not shell-wrapped runs an executable directly, where go's
// escaping is the correct escaping. Nothing may override it there.
func TestWindowsProcess_WithoutShellWrap_LeavesCmdLineEmpty(t *testing.T) {
	config := models.NewConfig([]string{"cmd.exe", "/C", "exit 0"})

	proc, err := newProcess(context.Background(), config)
	if err != nil {
		t.Fatalf("newProcess() error = %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := proc.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	wp, ok := proc.(*windowsProcess)
	if !ok {
		t.Fatalf("newProcess() returned %T, want *windowsProcess", proc)
	}
	if wp.cmd.SysProcAttr != nil && wp.cmd.SysProcAttr.CmdLine != "" {
		t.Errorf("CmdLine = %s, want empty so go escapes the arguments", wp.cmd.SysProcAttr.CmdLine)
	}
	if got := proc.ExitCode(); got != 0 {
		t.Errorf("ExitCode = %d, want 0", got)
	}
}

// Pinned against the real escaper rather than a copy of it: this is the
// transformation the fix exists to avoid, and the backslashes it inserts are
// exactly what ends up inside the path cmd then fails to find.
func TestWindowsShellCommandLine_GoEscapingWouldMangleTheProbe(t *testing.T) {
	escaped := syscall.EscapeArg(desktopProbe)

	if !strings.Contains(escaped, `\"`) {
		t.Fatalf("syscall.EscapeArg(%s) = %s, expected it to backslash-escape the quotes", desktopProbe, escaped)
	}
	if got := windowsShellCommandLine([]string{desktopProbe}); strings.Contains(got, `\"`) {
		t.Errorf("windowsShellCommandLine() = %s, want the command's own quotes left alone", got)
	}
}
