package unpack

import (
	"context"
	"io"
	"path/filepath"
	"strings"
)

const singleFilePerm = 0o755

func extractSingle(
	ctx context.Context,
	r io.Reader,
	name string,
	g *Guard,
) error {
	return g.file(ctx, name, singleFilePerm, r)
}

func singleName(
	path string,
) string {
	base := filepath.Base(path)

	return strings.TrimSuffix(base, filepath.Ext(base))
}
