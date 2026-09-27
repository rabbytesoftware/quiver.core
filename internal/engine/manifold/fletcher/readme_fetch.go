package fletcher

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const defaultReadmeKey = "README.md"

func defaultReadmeNames() []string {
	return []string{"README.md", "readme.md", "README.markdown", "README"}
}

func fetchReadme(
	ctx context.Context,
	host hosts.Forge,
	ns domain.Namespace,
	tag string,
) ([]byte, error) {
	_, raw, err := firstFile(ctx, host, ns, tag, defaultReadmeNames())
	if err != nil || raw == nil || !readme.IsCJKDominant(raw) {
		return raw, err
	}
	name, english, err := firstFile(ctx, host, ns, tag, readme.EnglishReadmeNames())
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
	host hosts.Forge,
	ns domain.Namespace,
	tag string,
	names []string,
) (string, []byte, error) {
	for _, name := range names {
		data, err := host.RawFile(ctx, ns, tag, name)
		if errors.Is(err, hosts.ErrRawNotFound) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("raw file %s: %w", name, err)
		}
		return name, data, nil
	}
	return "", nil, nil
}
