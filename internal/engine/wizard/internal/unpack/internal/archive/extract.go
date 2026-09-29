package archive

import (
	"archive/tar"
	"archive/zip"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
)

const (
	maxLinkTarget  = 4096
	singleFilePerm = 0o755
)

func (a *archive) extract(
	ctx context.Context,
	g *guard.Guard,
	singleFile string,
) error {
	if a.layout == layoutZip {
		return extractZip(ctx, a.src, a.size, g)
	}

	rc, err := a.codec.open(io.NewSectionReader(a.src, 0, a.size))
	if err != nil {
		return fmt.Errorf("unpack: %s: %w", a.src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	if a.layout == layoutTar {
		return extractTar(ctx, rc, g)
	}

	return g.File(ctx, cmp.Or(singleFile, singleName(a.src.Name())), singleFilePerm, rc)
}

type tarReader struct {
	tr *tar.Reader
	g  *guard.Guard
}

func extractTar(
	ctx context.Context,
	r io.Reader,
	g *guard.Guard,
) error {
	t := &tarReader{tr: tar.NewReader(r), g: g}

	for {
		done, err := t.next(ctx)
		if done || err != nil {
			return err
		}
	}
}

func (t *tarReader) next(
	ctx context.Context,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return true, fmt.Errorf("unpack: %w", err)
	}

	hdr, err := t.tr.Next()
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return true, fmt.Errorf("unpack: tar: %w", err)
	}

	return false, t.entry(ctx, hdr)
}

func (t *tarReader) entry(
	ctx context.Context,
	hdr *tar.Header,
) error {
	perm := hdr.FileInfo().Mode().Perm()

	switch hdr.Typeflag {
	case tar.TypeDir:
		return t.g.Dir(hdr.Name, perm)
	case tar.TypeReg:
		return t.g.File(ctx, hdr.Name, perm, t.tr)
	case tar.TypeSymlink:
		return t.g.Symlink(hdr.Name, hdr.Linkname)
	case tar.TypeLink:
		return t.g.Hardlink(hdr.Name, hdr.Linkname)
	}

	if err := t.g.Copy(ctx, io.Discard, t.tr); err != nil {
		return fmt.Errorf("unpack: skip %s: %w", hdr.Name, err)
	}

	return nil
}

func extractZip(
	ctx context.Context,
	src io.ReaderAt,
	size int64,
	g *guard.Guard,
) error {
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return fmt.Errorf("unpack: zip: %w", err)
	}

	for _, f := range zr.File {
		if err := zipEntry(ctx, f, g); err != nil {
			return err
		}
	}

	return nil
}

func zipEntry(
	ctx context.Context,
	f *zip.File,
	g *guard.Guard,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("unpack: %w", err)
	}

	mode := f.Mode()
	if mode.IsDir() {
		return g.Dir(f.Name, mode.Perm())
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("unpack: zip: %s: %w", f.Name, err)
	}
	defer rc.Close() //nolint:errcheck

	if mode&os.ModeSymlink != 0 {
		return zipSymlink(f.Name, rc, g)
	}

	return g.File(ctx, f.Name, mode.Perm(), rc)
}

func zipSymlink(
	name string,
	r io.Reader,
	g *guard.Guard,
) error {
	target, err := io.ReadAll(io.LimitReader(r, maxLinkTarget))
	if err != nil {
		return fmt.Errorf("unpack: zip: %s: %w", name, err)
	}

	return g.Symlink(name, string(target))
}

func singleName(
	path string,
) string {
	base := filepath.Base(path)

	return strings.TrimSuffix(base, filepath.Ext(base))
}
