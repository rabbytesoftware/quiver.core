package unpack

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
)

type tarReader struct {
	tr *tar.Reader
	g  *Guard
}

func extractTar(
	ctx context.Context,
	r io.Reader,
	g *Guard,
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
		return t.g.dir(hdr.Name, perm)
	case tar.TypeReg:
		return t.g.file(ctx, hdr.Name, perm, t.tr)
	case tar.TypeSymlink:
		return t.g.symlink(hdr.Name, hdr.Linkname)
	case tar.TypeLink:
		return t.g.hardlink(hdr.Name, hdr.Linkname)
	}

	if err := t.g.copy(ctx, io.Discard, t.tr); err != nil {
		return fmt.Errorf("unpack: skip %s: %w", hdr.Name, err)
	}

	return nil
}
