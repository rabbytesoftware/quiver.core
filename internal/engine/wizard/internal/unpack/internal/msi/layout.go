package msi

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const maxHoistDepth = 16

// layout maps an administrative image, which mirrors the package's Directory
// table (TARGETDIR\PFiles\Vendor\App\...) next to a copy of the package, onto
// the destination: the application directory's contents become the top level.
type layout struct {
	copy string
	app  string
}

func planLayout(
	image string,
	msiName string,
) (layout, error) {
	entries, err := os.ReadDir(image)
	if err != nil {
		return layout{}, fmt.Errorf("unpack: msi: read image: %w", err)
	}

	l := layout{copy: msiName}
	root := programFilesRoot(entries)
	if root == "" {
		return l, nil
	}

	l.app, err = descend(image, root)

	return l, err
}

func (l layout) route(
	_ string,
	rel string,
	d fs.DirEntry,
) (string, bool) {
	if !d.IsDir() && strings.EqualFold(rel, l.copy) {
		return "", false
	}
	if l.app == "" {
		return rel, true
	}

	inner, err := filepath.Rel(l.app, rel)
	switch {
	case err != nil:
		return rel, true
	case inner == ".":
		return "", true
	case workfs.RelInside(inner):
		return inner, true
	case workfs.Inside(rel, l.app):
		return "", true
	}

	return rel, true
}

func programFilesRoot(
	entries []os.DirEntry,
) string {
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 1 {
		return dirs[0]
	}

	for _, dir := range dirs {
		if isProgramFiles(dir) {
			return dir
		}
	}

	return ""
}

func isProgramFiles(
	name string,
) bool {
	switch strings.ToLower(name) {
	case "pfiles", "pfiles64", "program files", "program files (x86)",
		"programfilesfolder", "programfiles64folder", "programfiles6432folder":
		return true
	}

	return false
}

func descend(
	image string,
	rel string,
) (string, error) {
	for range maxHoistDepth {
		entries, err := os.ReadDir(filepath.Join(image, rel))
		if err != nil {
			return "", fmt.Errorf("unpack: msi: read image: %w", err)
		}
		if len(entries) != 1 || !entries[0].IsDir() {
			return rel, nil
		}
		rel = filepath.Join(rel, entries[0].Name())
	}

	return rel, nil
}
