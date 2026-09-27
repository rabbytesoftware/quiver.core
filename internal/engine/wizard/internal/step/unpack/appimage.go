package unpack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/squashfs"
)

var (
	ErrUnsupportedAppImage = errors.New("unpack: unsupported appimage type")
	ErrNoSquashfs          = errors.New("unpack: appimage has no squashfs image")
)

const (
	appImageSuffix = ".AppImage"
	appDirSuffix   = ".AppDir"
	appImageType2  = "AI\x02"
	squashfsMagic  = "hsqs"

	maxSquashfsDepth = 256
)

func AppDirName(
	from string,
) string {
	base := filepath.Base(from)
	stem := len(base) - len(appImageSuffix)
	if stem > 0 && strings.EqualFold(base[stem:], appImageSuffix) && isPathComponent(base[:stem]) {
		return base[:stem]
	}

	return base + appDirSuffix
}

func isPathComponent(
	name string,
) bool {
	return name != "." && name != ".."
}

func ExtractAppImage(
	ctx context.Context,
	src *os.File,
	size int64,
	g *Guard,
) error {
	if err := checkAppImageType(src); err != nil {
		return err
	}

	off, err := squashfsOffset(src)
	if err != nil {
		return err
	}

	found, err := hasSquashfsMagic(src, off)
	if err != nil {
		return fmt.Errorf("unpack: read %s: %w", src.Name(), err)
	}
	if !found {
		return fmt.Errorf("unpack: %s at offset %d: %w", src.Name(), off, ErrNoSquashfs)
	}

	sfs, err := squashfs.Read(file.New(src, true), size-off, off, 0)
	if err != nil {
		return fmt.Errorf("unpack: %s: squashfs: %w", src.Name(), err)
	}
	defer sfs.Close() //nolint:errcheck

	return walkSquashfs(ctx, sfs, ".", 0, g)
}

func hasSquashfsMagic(
	src io.ReaderAt,
	off int64,
) (bool, error) {
	magic := make([]byte, len(squashfsMagic))
	if _, err := src.ReadAt(magic, off); err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}

	return string(magic) == squashfsMagic, nil
}

func checkAppImageType(
	src *os.File,
) error {
	head := make([]byte, appImageMagicLen)
	if _, err := src.ReadAt(head, 0); err != nil {
		return fmt.Errorf("unpack: read %s: %w", src.Name(), err)
	}

	if tag := string(head[8:appImageMagicLen]); tag != appImageType2 {
		return fmt.Errorf("unpack: %s: type %q: %w", src.Name(), tag, ErrUnsupportedAppImage)
	}

	return nil
}

func walkSquashfs(
	ctx context.Context,
	sfs *squashfs.FileSystem,
	dir string,
	depth int,
	g *Guard,
) error {
	if depth > maxSquashfsDepth {
		return fmt.Errorf("unpack: squashfs: %s: nesting deeper than %d levels", dir, maxSquashfsDepth)
	}

	entries, err := sfs.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("unpack: squashfs: %w", err)
	}

	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("unpack: %w", err)
		}

		if err := squashfsEntry(ctx, sfs, dir, depth, e, g); err != nil {
			return err
		}
	}

	return nil
}

func squashfsEntry(
	ctx context.Context,
	sfs *squashfs.FileSystem,
	dir string,
	depth int,
	e fs.DirEntry,
	g *Guard,
) error {
	name, err := squashfsEntryName(dir, e.Name())
	if err != nil {
		return err
	}

	info, err := e.Info()
	if err != nil {
		return fmt.Errorf("unpack: squashfs: %s: %w", name, err)
	}

	mode := info.Mode()
	switch {
	case mode.IsDir():
		if err := g.dir(name, mode.Perm()); err != nil {
			return err
		}
		return walkSquashfs(ctx, sfs, name, depth+1, g)
	case mode&fs.ModeSymlink != 0:
		return squashfsSymlink(name, e, g)
	case mode.IsRegular():
		return squashfsFile(ctx, sfs, name, mode.Perm(), g)
	}

	return nil
}

func squashfsEntryName(
	dir string,
	base string,
) (string, error) {
	if base == "" || base == "." || base == ".." || strings.ContainsAny(base, "/\\\x00") {
		return "", fmt.Errorf("unpack: squashfs: %q in %s: invalid entry name: %w", base, dir, ErrEscape)
	}

	return path.Join(dir, base), nil
}

func squashfsSymlink(
	name string,
	e fs.DirEntry,
	g *Guard,
) error {
	link, ok := e.(interface{ Readlink() (string, error) })
	if !ok {
		return fmt.Errorf("unpack: squashfs: %s: unreadable symlink", name)
	}

	target, err := link.Readlink()
	if err != nil {
		return fmt.Errorf("unpack: squashfs: readlink %s: %w", name, err)
	}

	return g.symlink(name, target)
}

func squashfsFile(
	ctx context.Context,
	sfs *squashfs.FileSystem,
	name string,
	perm fs.FileMode,
	g *Guard,
) error {
	f, err := sfs.OpenFile(name, os.O_RDONLY)
	if err != nil {
		return fmt.Errorf("unpack: squashfs: open %s: %w", name, err)
	}
	defer f.Close() //nolint:errcheck

	return g.file(ctx, name, perm, f)
}
