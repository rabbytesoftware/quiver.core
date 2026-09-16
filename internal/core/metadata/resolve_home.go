//go:build !windows

package metadata

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// resolveHome expands the Unix home template into an absolute path.
// If os.UserHomeDir fails (e.g. HOME unset in a container), it logs a warning
// and returns the raw template value, which will produce a path relative to cwd.
func resolveHome() string {
	if override := os.Getenv(homeOverrideEnv); override != "" {
		return override
	}

	raw := Get().Paths.Home.Resolve()
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn(
				"cannot determine user home directory; Quiver files will be written to the raw path",
				"path", raw,
			)
			return raw
		}
		return filepath.Join(home, raw[2:])
	}
	return raw
}
