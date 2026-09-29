package gather

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const defaultReadmeKey = "README.md"

func defaultReadmeNames() []string {
	return []string{"README.md", "readme.md", "README.markdown", "README"}
}

func fetchReadme(
	ctx context.Context,
	fetch fetchFunc,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
) ([]byte, error) {
	_, raw, err := firstFile(ctx, fetch, host, ns, tag, defaultReadmeNames())
	if err != nil || raw == nil || !readme.IsCJKDominant(raw) {
		return raw, err
	}
	name, english, err := firstFile(ctx, fetch, host, ns, tag, readme.EnglishReadmeNames())
	if err != nil {
		return nil, err
	}
	byName := map[string][]byte{defaultReadmeKey: raw}
	if english != nil {
		byName[name] = english
	}
	return readme.SelectReadme(byName), nil
}

func firstFile(
	ctx context.Context,
	fetch fetchFunc,
	host hosts.Host,
	ns domain.Namespace,
	tag string,
	names []string,
) (string, []byte, error) {
	for _, name := range names {
		url, err := host.RawFileURL(ns, tag, name)
		if err != nil {
			return "", nil, fmt.Errorf("raw file url %s: %w", name, err)
		}
		data, err := fetch(ctx, url)
		if errors.Is(err, resolver.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("raw file %s: %w", name, err)
		}
		return name, data, nil
	}
	return "", nil, nil
}
