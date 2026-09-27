package unpack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	LauncherName = ".quiver-run"
	launcherPerm = 0o755
	launcherHead = "#!/bin/sh\n" +
		"APPDIR=$(dirname \"$(readlink -f \"$0\")\")\n" +
		"export APPDIR\n" +
		"exec \"$APPDIR/AppRun\""
)

func WriteLauncher(
	appDir string,
	args []string,
) (string, error) {
	abs, err := filepath.Abs(appDir)
	if err != nil {
		return "", fmt.Errorf("unpack: launcher: %w", err)
	}

	root, err := os.OpenRoot(abs)
	if err != nil {
		return "", fmt.Errorf("unpack: launcher: %w", err)
	}
	defer root.Close() //nolint:errcheck

	_ = root.Remove(LauncherName)
	if err := writeExecutable(root, LauncherName, []byte(launcherScript(args))); err != nil {
		return "", fmt.Errorf("unpack: launcher: %w", err)
	}

	return filepath.Join(abs, LauncherName), nil
}

func writeExecutable(
	root *os.Root,
	name string,
	data []byte,
) error {
	if err := root.WriteFile(name, data, launcherPerm); err != nil {
		return err
	}

	return root.Chmod(name, launcherPerm)
}

func launcherScript(
	args []string,
) string {
	var b strings.Builder
	b.WriteString(launcherHead)
	for _, arg := range args {
		b.WriteString(" ")
		b.WriteString(shellQuote(arg))
	}
	b.WriteString(" \"$@\"\n")

	return b.String()
}

func shellQuote(
	s string,
) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
