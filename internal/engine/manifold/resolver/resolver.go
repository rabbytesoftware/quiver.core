package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

const defaultFetchTimeout = 30 * time.Second

type Resolver interface {
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) ([]byte, string, error)

	ResolveCollection(
		ctx context.Context,
		namespace domain.Namespace,
	) ([]byte, error)
}

// Resolver converts a namespace into YAML bytes by resolving the namespace
// to a git clone URL and file path, then fetching the file from the repository.
type resolver struct {
	timeout  time.Duration
	fetchers []resolvers.Fetcher
}

// New builds a resolver that tries each host's raw-file URL before cloning.
// A namespace on a host the lookup does not know still resolves: git needs no
// host knowledge.
func New(
	timeout time.Duration,
	lookup hosts.Lookup,
) Resolver {
	if timeout == 0 {
		timeout = defaultFetchTimeout
	}

	return &resolver{
		timeout: timeout,
		fetchers: []resolvers.Fetcher{
			resolvers.NewHTTP(lookup),
			resolvers.NewGit(),
		},
	}
}

func (r *resolver) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, string, error) {
	filePaths, err := resolveArrowParts(namespace)
	if err != nil {
		return nil, "", err
	}

	return r.fetchManifest(ctx, namespace, filePaths)
}

func (r *resolver) ResolveCollection(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, error) {
	if err := namespace.BareNamespace().Validate(); err != nil {
		return nil, fmt.Errorf("resolver: invalid namespace: %w", err)
	}

	data, _, err := r.fetchManifest(ctx, namespace, []string{"COLLECTION.md", "collection.yaml"})
	return data, err
}

func (r *resolver) fetchManifest(
	ctx context.Context,
	namespace domain.Namespace,
	filePaths []string,
) ([]byte, string, error) {
	var lastErr error

	for _, filePath := range filePaths {
		for _, f := range r.fetchers {
			if !f.CanResolve(namespace) {
				continue
			}
			data, err := f.Fetch(ctx, namespace, filePath, r.timeout)
			if err == nil {
				return data, filePath, nil
			}
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, "", lastErr
	}

	return nil, "", fmt.Errorf("%w: no fetcher could resolve %s", resolvers.ErrFetchFailed, namespace)
}

func resolveArrowParts(
	namespace domain.Namespace,
) ([]string, error) {
	bare := namespace.BareNamespace()
	if err := bare.Validate(); err != nil {
		return nil, fmt.Errorf("resolver: invalid namespace: %w", err)
	}

	auid := bare.GetAUID()
	if auid != "" {
		return []string{auid + ".md", auid + ".yaml"}, nil
	}

	return []string{"ARROW.md", "arrow.yaml"}, nil
}
