package portable

import (
	"context"
	"errors"
	"os"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
)

func (h *handler) installDmg(
	ctx context.Context,
	from string,
	to string,
) ([]domain.PortableApp, error) {
	return h.installBundles(ctx, to, func(g *unpack.Guard) error {
		return unpack.ExtractDmg(ctx, from, g)
	})
}

func (h *handler) installArchive(
	ctx context.Context,
	osArch domain.OS,
	src *os.File,
	size int64,
	archive unpack.Archive,
	to string,
) ([]domain.PortableApp, error) {
	extract := func(g *unpack.Guard) error {
		return archive.Extract(ctx, src, size, g)
	}
	if osArch.IsDarwin() {
		return h.installBundles(ctx, to, extract)
	}

	_, err := h.unpackInto(ctx, to, extract)

	return nil, err
}

func (h *handler) installBundles(
	ctx context.Context,
	to string,
	extract func(*unpack.Guard) error,
) ([]domain.PortableApp, error) {
	tops, err := h.unpackInto(ctx, to, extract)
	if err != nil {
		return nil, err
	}

	return bundleApps(to, tops), nil
}

func (h *handler) unpackInto(
	ctx context.Context,
	to string,
	extract func(*unpack.Guard) error,
) ([]string, error) {
	g, err := unpack.OpenGuard(ctx, to, h.maxBytes)
	if err != nil {
		return nil, err
	}
	defer g.Close()

	if err := errors.Join(extract(g), g.Verify()); err != nil {
		return nil, err
	}

	return g.TopLevel(), nil
}
