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

	// ResolveArrowAt fetches a manifest at an explicit path within the
	// namespace's repository (markdown tried before YAML), for a
	// quiver-hosted arrow whose location was already looked up in its
	// owning collection.
	ResolveArrowAt(
		ctx context.Context,
		namespace domain.Namespace,
		path string,
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
	if err := namespace.BareNamespace().Validate(); err != nil {
		return nil, "", fmt.Errorf("resolver: invalid namespace: %w", err)
	}
	return r.fetchManifest(ctx, namespace, []string{"ARROW.md", "arrow.yaml"})
}

func (r *resolver) ResolveArrowAt(
	ctx context.Context,
	namespace domain.Namespace,
	path string,
) ([]byte, string, error) {
	if err := namespace.BareNamespace().Validate(); err != nil {
		return nil, "", fmt.Errorf("resolver: invalid namespace: %w", err)
	}
	if path == "" {
		return nil, "", fmt.Errorf("resolver: resolve arrow at %s: empty path", namespace)
	}
	return r.fetchManifest(ctx, namespace, []string{path + ".md", path + ".yaml"})
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

// fetchManifest tries each fetcher in turn, handing it the full candidate
// list in one call rather than looping over candidates itself. This means
// the global trying order is "every candidate via HTTP, then every
// candidate via git" rather than the reverse nesting ("ARROW.md via every
// fetcher, then arrow.yaml via every fetcher") — a deliberate choice, not a
// side effect: HTTP has no per-attempt cost, so exhausting it fully before
// ever paying for a git clone is strictly better, and no candidate-vs-host
// ordering here carries any correctness meaning of its own.
func (r *resolver) fetchManifest(
	ctx context.Context,
	namespace domain.Namespace,
	filePaths []string,
) ([]byte, string, error) {
	var lastErr error

	for _, f := range r.fetchers {
		if !f.CanResolve(namespace) {
			continue
		}
		data, matchedPath, err := f.Fetch(ctx, namespace, filePaths, r.timeout)
		if err == nil {
			return data, matchedPath, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, "", lastErr
	}

	return nil, "", fmt.Errorf("%w: no fetcher could resolve %s", resolvers.ErrFetchFailed, namespace)
}
