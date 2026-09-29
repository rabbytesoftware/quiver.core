package appimage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
)

const (
	appRunName    = "AppRun"
	desktopSuffix = ".desktop"
)

type Meta struct {
	Name string
	Args []string
	Icon string
}

func ReadMeta(
	appDir string,
) (Meta, error) {
	root, err := os.OpenRoot(appDir)
	if err != nil {
		return Meta{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}
	defer root.Close() //nolint:errcheck

	entry, err := readDesktopEntry(root)
	if err != nil {
		return Meta{}, err
	}

	return Meta{
		Name: entry.name,
		Args: vendorArgs(root, entry.exec),
		Icon: resolveIcon(root, entry.icon),
	}, nil
}

func readDesktopEntry(
	root *os.Root,
) (desktopEntry, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return desktopEntry{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), desktopSuffix) || !isRegularInside(root, e.Name()) {
			continue
		}

		return readDesktopFile(root, e.Name())
	}

	return desktopEntry{}, nil
}

func readDesktopFile(
	root *os.Root,
	name string,
) (desktopEntry, error) {
	f, err := root.Open(name)
	if err != nil {
		return desktopEntry{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}
	defer f.Close() //nolint:errcheck

	return readCappedDesktopFile(f)
}

func vendorArgs(
	root *os.Root,
	exec string,
) []string {
	program, args := parseExec(exec)
	if program == appRunName || isRegularInside(root, program) {
		return args
	}

	return nil
}

func isRegularInside(
	root *os.Root,
	name string,
) bool {
	if name == "" || guard.IsAbsolute(name) {
		return false
	}

	info, err := root.Stat(filepath.FromSlash(name))

	return err == nil && info.Mode().IsRegular()
}
