package unpack

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	appRunName    = "AppRun"
	desktopSuffix = ".desktop"
)

type AppImageMeta struct {
	Name string
	Args []string
	Icon string
}

func ReadAppImageMeta(
	appDir string,
) (AppImageMeta, error) {
	root, err := os.OpenRoot(appDir)
	if err != nil {
		return AppImageMeta{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}
	defer root.Close() //nolint:errcheck

	entry, err := readDesktopEntry(root)
	if err != nil {
		return AppImageMeta{}, err
	}

	return AppImageMeta{
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
	if name == "" || isAbsolute(name) {
		return false
	}

	info, err := root.Stat(filepath.FromSlash(name))

	return err == nil && info.Mode().IsRegular()
}
