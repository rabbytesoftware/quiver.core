package launch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const markerName = ".quiver-launch"

// Record stores target as the thing opening the arrow installed in workdir
// should start. It is a one-line file so a launch needs no manifest or catalog
// read.
func Record(
	workdir string,
	target string,
) error {
	return os.WriteFile(filepath.Join(workdir, markerName), []byte(target+"\n"), 0o600)
}

// Recorded returns the target Record stored for workdir, or "" when the arrow
// has none.
func Recorded(
	workdir string,
) (string, error) {
	data, err := os.ReadFile(filepath.Join(workdir, markerName)) // #nosec G304 -- path is the shelf's own marker inside a validated workdir
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Clear drops the target recorded for workdir; a workdir without one is not an
// error.
func Clear(
	workdir string,
) error {
	err := os.Remove(filepath.Join(workdir, markerName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
