package process

import "strings"

const (
	// windowsShell is the interpreter a shell-wrapped step runs under on
	// windows, and the argv[0] token of the command line built for it.
	windowsShell = "cmd.exe"
	// windowsShellFlag tells cmd to run the rest of the line and exit.
	windowsShellFlag = "/C"
)

// windowsShellCommandLine returns the literal command line for a
// shell-wrapped step on windows, assigned to syscall.SysProcAttr.CmdLine.
//
// Left empty, syscall.StartProcess builds the line with makeCmdLine, whose
// escaping targets MSVCRT argv rules that cmd.exe does not follow: cmd reads
// a backslash as a path separator, never a quote escape, so a probe like
//
//	if exist "%ProgramFiles%\Quiver\quiverdesktop.exe" (exit 0) else (exit 1)
//
// comes back as \"%ProgramFiles%\...\quiverdesktop.exe\", with the escape
// backslashes now part of the tested path -- the path never matches and
// nothing reports an error.
//
// The command is bracketed in one quote pair rather than pasted bare: per
// the second quote rule in `cmd /?`, cmd strips exactly that pair when the
// text after /C opens with a quote it does not preserve. Pasted bare, the
// same rule would instead eat a pair belonging to the command itself.
func windowsShellCommandLine(
	command []string,
) string {
	return windowsShell + " " + windowsShellFlag + ` "` + strings.Join(command, " ") + `"`
}
