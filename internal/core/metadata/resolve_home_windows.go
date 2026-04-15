//go:build windows

package metadata

import (
	"os/user"
	"strings"
)

// resolveHome expands the Windows home template into an absolute path,
// substituting {{USER}} with the current OS username.
func resolveHome() string {
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
