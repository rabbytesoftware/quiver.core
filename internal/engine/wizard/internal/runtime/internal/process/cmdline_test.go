package process

import (
	"strings"
	"testing"
)

// desktopProbe is the shape quiver.desktop's own `preinstalled` check needs:
// an install path that routinely contains spaces (%ProgramFiles%, or
// %LOCALAPPDATA% under an account named "John Smith"), so the quotes around it
// are load-bearing, wrapped in the if/else batch syntax whose parentheses cmd
// treats as special characters.
const desktopProbe = `if exist "%ProgramFiles%\Quiver\quiverdesktop.exe" (exit 0) else (exit 1)`

func TestWindowsShellCommandLine_QuotedPathKeepsItsQuotes(t *testing.T) {
	got := windowsShellCommandLine([]string{desktopProbe})

	want := `cmd.exe /C "if exist "%ProgramFiles%\Quiver\quiverdesktop.exe" (exit 0) else (exit 1)"`
	if got != want {
		t.Errorf("windowsShellCommandLine() =\n\t%s\nwant\n\t%s", got, want)
	}
}

// Everything the fix promises reduces to this invariant: the command quiver
// was handed appears in the command line as one unbroken run of bytes, with a
// single quote pair around it and nothing else added or escaped. That pair is
// the one cmd strips back off under the second rule in `cmd /?`, so what the
// command processor finally sees is the manifest's own text.
func TestWindowsShellCommandLine_WrapsCommandInOneQuotePair(t *testing.T) {
	testCases := []struct {
		name    string
		command []string
	}{
		{name: "quoted path probe", command: []string{desktopProbe}},
		{name: "quoted path with arguments", command: []string{`"%ProgramFiles%\Quiver\q.exe" --check "C:\a b"`}},
		{name: "unquoted simple command", command: []string{"echo hello"}},
		{name: "bare executable", command: []string{"quiverdesktop.exe"}},
		{name: "unquoted windows path", command: []string{`C:\Quiver\quiverdesktop.exe --version`}},
		{name: "trailing backslash", command: []string{`dir C:\Program Files\Quiver\`}},
		{name: "cmd special characters", command: []string{`echo a & echo b | findstr b`}},
		{name: "multi element command", command: []string{"echo", "hello", "world"}},
		{name: "empty command", command: nil},
	}

	const prefix = windowsShell + " " + windowsShellFlag + ` "`

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := windowsShellCommandLine(tc.command)

			if !strings.HasPrefix(got, prefix) {
				t.Fatalf("windowsShellCommandLine() = %s, want prefix %s", got, prefix)
			}
			if !strings.HasSuffix(got, `"`) {
				t.Fatalf("windowsShellCommandLine() = %s, want a trailing quote", got)
			}

			inner := got[len(prefix) : len(got)-1]
			if want := strings.Join(tc.command, " "); inner != want {
				t.Errorf("command reached cmd as\n\t%s\nwant\n\t%s", inner, want)
			}
		})
	}
}

// The wrapper adds its quote pair and nothing more: no doubled backslash, no
// escape character that cmd would pass through as a literal.
func TestWindowsShellCommandLine_AddsNoEscapeCharacters(t *testing.T) {
	command := `dir "C:\Program Files\Quiver\"`

	got := windowsShellCommandLine([]string{command})

	if strings.Contains(got, `\\`) {
		t.Errorf("windowsShellCommandLine() = %s, want no doubled backslash", got)
	}
	if want := len(command) + len(windowsShell) + len(windowsShellFlag) + 4; len(got) != want {
		t.Errorf("windowsShellCommandLine() length = %d, want %d (the command plus one quote pair)", len(got), want)
	}
}
