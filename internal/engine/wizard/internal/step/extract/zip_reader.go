package extract

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
)

const maxLinkTarget = 4096

func extractZip(
	ctx context.Context,
	src io.ReaderAt,
	size int64,
	g *guard,
) error {
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return fmt.Errorf("extract: zip: %w", err)
	}

	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract: %w", err)
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
	g *guard,
) error {
	mode := f.Mode()
	if mode.IsDir() {
		return g.dir(f.Name, mode.Perm())
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("extract: zip: %s: %w", f.Name, err)
	}
	defer rc.Close() //nolint:errcheck

	if mode&os.ModeSymlink != 0 {
		return zipSymlink(f.Name, rc, g)
	}

	return g.file(ctx, f.Name, mode.Perm(), rc)
}

func zipSymlink(
	name string,
	r io.Reader,
	g *guard,
) error {
	target, err := io.ReadAll(io.LimitReader(r, maxLinkTarget))
	if err != nil {
		return fmt.Errorf("extract: zip: %s: %w", name, err)
	}

	return g.symlink(name, string(target))
}
