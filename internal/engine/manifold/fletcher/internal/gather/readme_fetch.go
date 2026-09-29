package gather

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

func defaultReadmeNames() []string {
	return []string{"README.md", "readme.md", "README.markdown", "README"}
}

func fetchReadme(
	ctx context.Context,
	fetch media.Fetch,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
) ([]byte, error) {
	raw, err := firstFile(ctx, fetch, host, ns, tag, defaultReadmeNames())
	if err != nil || raw == nil || !readme.IsCJKDominant(raw) {
		return raw, err
	}
	english, err := firstFile(ctx, fetch, host, ns, tag, readme.EnglishReadmeNames())
	if err != nil {
		return nil, err
	}
	if english == nil {
		return raw, nil
	}
	return english, nil
}

func firstFile(
	ctx context.Context,
	fetch media.Fetch,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
	names []string,
) ([]byte, error) {
	for _, name := range names {
		url, err := host.RawFileURL(ns, tag, name)
		if err != nil {
			return nil, fmt.Errorf("raw file url %s: %w", name, err)
		}
		data, err := fetch(ctx, url)
		if errors.Is(err, resolver.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("raw file %s: %w", name, err)
		}
		return data, nil
	}
	return nil, nil
}
