package resolver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

const defaultFetchTimeout = 30 * time.Second

type Resolver interface {
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) ([]byte, error)

	ResolveQuiver(
		ctx context.Context,
		namespace domain.Namespace,
	) ([]byte, error)
}

type fetcher func(
	ctx context.Context,
	cloneURL string,
	filePath string,
	timeout time.Duration,
) ([]byte, error)

// Resolver converts a namespace into YAML bytes by resolving the namespace
// to a git clone URL and file path, then fetching the file from the repository.
type resolver struct {
	timeout time.Duration
	fetch   fetcher
}

func New(
	timeout time.Duration,
) Resolver {
	if timeout == 0 {
		timeout = defaultFetchTimeout
	}

	return &resolver{
		timeout: timeout,
		fetch:   fetchFile,
	}
}

func (r *resolver) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, error) {
	cloneURL, filePath, err := resolveArrowParts(namespace)
	if err != nil {
		return nil, err
	}

	return r.fetch(ctx, cloneURL, filePath, r.timeout)
}

func (r *resolver) ResolveQuiver(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, error) {
	cloneURL, _, err := resolve(namespace)
	if err != nil {
		return nil, err
	}

	return r.fetch(ctx, cloneURL, "quiver.yaml", r.timeout)
}

func resolveArrowParts(
	namespace domain.Namespace,
) (string, string, error) {
	cloneURL, parts, err := resolve(namespace)
	if err != nil {
		return "", "", err
	}

	if len(parts) == 4 {
		return cloneURL, parts[3] + ".yaml", nil
	}

	return cloneURL, "arrow.yaml", nil
}

func resolve(
	namespace domain.Namespace,
) (string, []string, error) {
	if err := namespace.Validate(); err != nil {
		return "", nil, fmt.Errorf("resolver: invalid namespace: %w", err)
	}

	parts := strings.Split(string(namespace), domain.NamespaceSeparator)

	cloneURL := fmt.Sprintf("https://%s/%s/%s", parts[0], parts[1], parts[2])

	return cloneURL, parts, nil
}
