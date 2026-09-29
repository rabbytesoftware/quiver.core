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

func (a Archive) Extract(
	ctx context.Context,
	src *os.File,
	size int64,
	g *guard.Guard,
	singleFile string,
) error {
	switch a.layout {
	case layoutZip:
		return extractZip(ctx, src, size, g)
	case layoutTar, layoutSingle:
	}

	rc, err := a.codec.open(io.NewSectionReader(src, 0, size))
	if err != nil {
		return fmt.Errorf("unpack: %s: %w", src.Name(), err)
	}
	defer rc.Close() //nolint:errcheck

	if a.layout == layoutTar {
		return extractTar(ctx, rc, g)
	}

	return extractSingle(ctx, rc, cmp.Or(singleFile, singleName(src.Name())), g)
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
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("unpack: %w", err)
		}

		hdr, err := t.tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("unpack: tar: %w", err)
		}

		if err := t.entry(ctx, hdr); err != nil {
			return err
		}
	}
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

const maxLinkTarget = 4096

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
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("unpack: %w", err)
		}

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

const singleFilePerm = 0o755

func extractSingle(
	ctx context.Context,
	r io.Reader,
	name string,
	g *guard.Guard,
) error {
	return g.File(ctx, name, singleFilePerm, r)
}

func singleName(
	path string,
) string {
	base := filepath.Base(path)

	return strings.TrimSuffix(base, filepath.Ext(base))
}
