package extract

import (
	"context"
	"io"
)

const MaxEntries = maxEntries

func ExtractTarWithEntryLimit(
	ctx context.Context,
	r io.Reader,
	dest string,
	limit int,
) error {
	g, err := openGuard(ctx, dest, 1<<20)
	if err != nil {
		return err
	}
	defer g.close()
	g.maxEntries = limit
	return extractTar(ctx, r, g)
}
