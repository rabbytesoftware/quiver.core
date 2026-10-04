//go:build windows

package metadata

import (
	"log/slog"
	"os"
)

// resolveHome expands the Windows home template into an absolute path using
// the real profile directory. user.Current().Username is not usable here: on
// Windows it is COMPUTERNAME\name or DOMAIN\name, not a directory name.
// If the profile directory cannot be determined it logs a warning and expands
// to a path relative to the working directory.
func resolveHome() string {
	if override := os.Getenv(homeOverrideEnv); override != "" {
		return override
	}

	raw := Get().Paths.Home.Resolve()
	profile, err := os.UserHomeDir()
	if err != nil {
		slog.Warn(
			"cannot determine user profile directory; Quiver files will be written relative to the working directory",
			"path", raw,
		)
		profile = "."
	}
	return expandProfile(raw, profile)
}
