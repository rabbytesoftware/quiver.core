package process

import "strings"

const (
	// windowsShell is the interpreter a shell-wrapped step runs under on
	// windows, and the argv[0] token of the command line built for it.
	windowsShell = "cmd.exe"
	// windowsShellFlag tells cmd to run the rest of the line and exit.
	windowsShellFlag = "/C"
)

// windowsShellCommandLine returns the literal command line for a shell-wrapped
// step on windows, to be assigned to syscall.SysProcAttr.CmdLine.
//
// This lives outside windows.go, and carries no build constraint, so the string
// it produces can be asserted from any host. The string is the whole fix, and a
// fix to process spawning on every windows install in the ecosystem cannot be
// left to a machine none of the other tests in this package run on.
//
// Setting CmdLine is what makes the difference: left empty, syscall.StartProcess
// builds the line with makeCmdLine, whose escaping targets the MSVCRT argv rules
// that cmd.exe does not follow. cmd reads a backslash as a path separator and
// never as a quote escape, so a probe like
//
//	if exist "%ProgramFiles%\Quiver\quiverdesktop.exe" (exit 0) else (exit 1)
//
// comes back out of that round trip as \"%ProgramFiles%\...\quiverdesktop.exe\",
// with the escape backslashes now part of the path being tested. The path never
// matches, the probe reports "not installed", and nothing anywhere reports an
// error. Parentheses are cmd special characters, which is what rules out the one
// quote-preserving case in `cmd /?` and lets the mangling through.
//
// The command is bracketed in one quote pair rather than pasted in bare. Per the
// second quote rule in `cmd /?`, when the text after /C opens with a quote and
// does not qualify for preservation, cmd strips that first character and the
// last quote character on the line: exactly the pair added here, leaving the
// command verbatim. Pasted in bare, that same rule would instead eat a pair
// belonging to the command itself whenever it opens with a quoted path, as in
// `"%ProgramFiles%\Quiver\q.exe" --check "C:\a b"`.
func windowsShellCommandLine(
	command []string,
) string {
	return windowsShell + " " + windowsShellFlag + ` "` + strings.Join(command, " ") + `"`
}
