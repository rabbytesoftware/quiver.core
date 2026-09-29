package workfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

const AsideSuffix = ".quiver-old"

func Inside(
	dir string,
	path string,
) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && RelInside(rel)
}

func RelInside(
	rel string,
) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Relocate(
	path string,
	from string,
	to string,
) string {
	rel, err := filepath.Rel(from, path)
	if path == "" || err != nil || !RelInside(rel) {
		return path
	}
	return filepath.Join(to, rel)
}

// Swap moves staged into dest. A previous dest is only renamed to aside, never
// deleted, so a failed move into place can put it back; the caller removes
// aside once the swap succeeded.
func Swap(
	staged string,
	dest string,
	aside string,
	rename func(string, string) error,
) error {
	err := rename(dest, aside)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("move %s aside: %w", dest, err)
	}
	previous := err == nil

	if err := rename(staged, dest); err != nil {
		moveErr := fmt.Errorf("move %s into place: %w", dest, err)
		if previous {
			return errors.Join(moveErr, rename(aside, dest))
		}
		return moveErr
	}
	return nil
}
