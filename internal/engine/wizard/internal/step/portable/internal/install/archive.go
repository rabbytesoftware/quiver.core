package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

func (i *installer) installDmg(
	ctx context.Context,
	from string,
	to string,
) ([]domain.PortableApp, error) {
	return i.installBundles(ctx, to, func(g *unpack.Guard) error {
		return unpack.ExtractDmg(ctx, from, g)
	})
}

func (i *installer) installArchive(
	ctx context.Context,
	osArch domain.OS,
	src *os.File,
	size int64,
	archive unpack.Archive,
	to string,
	name string,
) ([]domain.PortableApp, error) {
	extract := func(g *unpack.Guard) error {
		return archive.Extract(ctx, src, size, g, name)
	}
	if osArch.IsDarwin() {
		return i.installBundles(ctx, to, extract)
	}

	_, err := i.unpackInto(ctx, to, extract)

	return nil, err
}

func (i *installer) installBundles(
	ctx context.Context,
	to string,
	extract func(*unpack.Guard) error,
) ([]domain.PortableApp, error) {
	tops, err := i.unpackInto(ctx, to, extract)
	if err != nil {
		return nil, err
	}

	return bundleApps(to, tops), nil
}

func (i *installer) unpackInto(
	ctx context.Context,
	to string,
	extract func(*unpack.Guard) error,
) ([]string, error) {
	g, err := unpack.OpenGuard(ctx, to, i.maxBytes)
	if err != nil {
		return nil, err
	}
	defer g.Close()

	if err := errors.Join(extract(g), g.Verify()); err != nil {
		return nil, err
	}

	return g.TopLevel(), nil
}

const bundleSuffix = ".app"

func bundleApps(
	to string,
	names []string,
) []domain.PortableApp {
	apps := make([]domain.PortableApp, 0, len(names))
	for _, name := range names {
		entry := filepath.Join(to, name)
		if !strings.HasSuffix(name, bundleSuffix) || !isDir(entry) {
			continue
		}
		apps = append(apps, domain.PortableApp{
			Name:  strings.TrimSuffix(name, bundleSuffix),
			Entry: entry,
		})
	}

	return apps
}

func isDir(
	path string,
) bool {
	info, err := os.Lstat(path)

	return err == nil && info.IsDir()
}
