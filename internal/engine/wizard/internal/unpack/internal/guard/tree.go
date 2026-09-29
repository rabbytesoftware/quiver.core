package guard

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
)

// Route names where a walked entry lands below the destination. keep=false
// prunes the entry and its subtree; an empty name walks into a directory
// without creating it.
type Route func(
	path string,
	rel string,
	d fs.DirEntry,
) (name string, keep bool)

type treeCopier struct {
	g     *Guard
	root  string
	route Route
}

func (g *Guard) CopyTree(
	ctx context.Context,
	root string,
	route Route,
) error {
	c := &treeCopier{g: g, root: root, route: route}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return c.visit(ctx, path, d, err)
	})
}

func (g *Guard) Apps(
	dir string,
	suffix string,
	dirs bool,
) []models.App {
	apps := []models.App{}
	for _, name := range g.TopLevel() {
		entry := filepath.Join(dir, name)
		if !hasSuffixFold(name, suffix) || !isKind(entry, dirs) {
			continue
		}
		apps = append(apps, models.App{Name: name[:len(name)-len(suffix)], Entry: entry})
	}

	return apps
}

func (c *treeCopier) visit(
	ctx context.Context,
	path string,
	d fs.DirEntry,
	err error,
) error {
	if err != nil {
		return fmt.Errorf("unpack: copy: %w", err)
	}

	rel, err := filepath.Rel(c.root, path)
	if err != nil || rel == "." {
		return err
	}

	name, keep := c.route(path, rel, d)
	if !keep {
		return prune(d)
	}
	if name == "" {
		return nil
	}

	return c.copy(ctx, path, name, d)
}

func (c *treeCopier) copy(
	ctx context.Context,
	path string,
	name string,
	d fs.DirEntry,
) error {
	switch {
	case d.Type()&fs.ModeSymlink != 0:
		return c.symlink(path, name)
	case d.IsDir():
		return c.g.Dir(name, DirPerm)
	case d.Type().IsRegular():
		return c.file(ctx, path, name, d)
	}

	return nil
}

func (c *treeCopier) symlink(
	path string,
	name string,
) error {
	target, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("unpack: copy: readlink %s: %w", name, err)
	}

	return c.g.Symlink(name, target)
}

func (c *treeCopier) file(
	ctx context.Context,
	path string,
	name string,
	d fs.DirEntry,
) error {
	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("unpack: copy: stat %s: %w", name, err)
	}

	rc, err := fns.ReadStream(ctx, path)
	if err != nil {
		return fmt.Errorf("unpack: copy: open %s: %w", name, err)
	}
	defer rc.Close() //nolint:errcheck

	return c.g.File(ctx, name, info.Mode().Perm(), rc)
}

func prune(
	d fs.DirEntry,
) error {
	if d.IsDir() {
		return fs.SkipDir
	}

	return nil
}

func hasSuffixFold(
	name string,
	suffix string,
) bool {
	return len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}

func isKind(
	path string,
	dir bool,
) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if dir {
		return info.IsDir()
	}

	return info.Mode().IsRegular()
}
