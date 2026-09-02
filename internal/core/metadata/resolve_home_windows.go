//go:build windows

package metadata

import (
	"os"
	"os/user"
	"strings"
)

// resolveHome expands the Windows home template into an absolute path,
// substituting {{USER}} with the current OS username.
func resolveHome() string {
	if override := os.Getenv(homeOverrideEnv); override != "" {
		return override
	}

	raw := Get().Paths.Home.Resolve()
	return strings.ReplaceAll(raw, "{{USER}}", currentUsername())
}

// currentUsername returns the current OS username for Windows home path expansion.
func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}
