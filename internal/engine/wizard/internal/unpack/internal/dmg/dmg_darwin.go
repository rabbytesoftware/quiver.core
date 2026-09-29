package dmg

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
)

const (
	detachTimeout = 30 * time.Second
)

type dmgCopier struct {
	mount string
	g     *guard.Guard
}

func Extract(
	ctx context.Context,
	image string,
	g *guard.Guard,
) error {
	mount, err := os.MkdirTemp("", "quiver-dmg-")
	if err != nil {
		return fmt.Errorf("unpack: dmg: mountpoint: %w", err)
	}
	defer os.Remove(mount) //nolint:errcheck

	if err := attachDmg(ctx, image, mount); err != nil {
		return err
	}
	defer detachDmg(ctx, mount)

	c := &dmgCopier{mount: mount, g: g}

	return filepath.WalkDir(mount, func(path string, d fs.DirEntry, err error) error {
		return c.visit(ctx, path, d, err)
	})
}

func attachDmg(
	ctx context.Context,
	image string,
	mount string,
) error {
	cmd := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-noautoopen", "-readonly", "-mountpoint", mount, image) // #nosec G204 -- fixed hdiutil binary; image is a workdir-resolved path and mount is a Quiver temp dir
	cmd.Stdin = strings.NewReader("Y\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unpack: dmg: attach %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func detachDmg(
	ctx context.Context,
	mount string,
) {
	detachCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachTimeout)
	defer cancel()

	out, err := exec.CommandContext(detachCtx, "hdiutil", "detach", mount, "-force").CombinedOutput() // #nosec G204 -- fixed hdiutil binary; mount is a Quiver temp dir
	if err != nil {
		slog.WarnContext(ctx, "unpack: dmg: detach failed", "mount", mount, "err", err, "output", string(out))
	}
}

func (c *dmgCopier) visit(
	ctx context.Context,
	path string,
	d fs.DirEntry,
	err error,
) error {
	if err != nil {
		return fmt.Errorf("unpack: dmg: %w", err)
	}

	rel := strings.TrimPrefix(strings.TrimPrefix(path, c.mount), string(filepath.Separator))
	if rel == "" {
		return nil
	}

	if c.skip(path, rel, d) {
		return skipEntry(d)
	}

	switch {
	case d.Type()&fs.ModeSymlink != 0:
		return c.symlink(path, rel)
	case d.IsDir():
		return c.g.Dir(rel, guard.DirPerm)
	case d.Type().IsRegular():
		return c.file(ctx, path, rel, d)
	}

	return nil
}

func (c *dmgCopier) skip(
	path string,
	rel string,
	d fs.DirEntry,
) bool {
	if strings.ContainsRune(rel, filepath.Separator) {
		return false
	}

	if strings.HasPrefix(rel, ".") {
		return true
	}

	if d.Type()&fs.ModeSymlink == 0 {
		return false
	}

	target, err := os.Readlink(path)

	return err == nil && filepath.IsAbs(target)
}

func (c *dmgCopier) symlink(
	path string,
	rel string,
) error {
	target, err := os.Readlink(path)
	if err != nil {
		return fmt.Errorf("unpack: dmg: readlink %s: %w", rel, err)
	}

	return c.g.Symlink(rel, target)
}

func (c *dmgCopier) file(
	ctx context.Context,
	path string,
	rel string,
	d fs.DirEntry,
) error {
	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("unpack: dmg: stat %s: %w", rel, err)
	}

	rc, err := fns.ReadStream(ctx, path)
	if err != nil {
		return fmt.Errorf("unpack: dmg: open %s: %w", rel, err)
	}
	defer rc.Close() //nolint:errcheck

	return c.g.File(ctx, rel, info.Mode().Perm(), rc)
}

func skipEntry(
	d fs.DirEntry,
) error {
	if d.IsDir() {
		return fs.SkipDir
	}

	return nil
}
