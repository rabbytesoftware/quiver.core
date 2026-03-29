package resolver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

const defaultFetchTimeout = 30 * time.Second

var supportedPlatforms = map[string]struct{}{
	"github.com":    {},
	"gitlab.com":    {},
	"bitbucket.org": {},
}

type fetcher func(
	ctx context.Context,
	cloneURL string,
	filePath string,
	timeout time.Duration,
) ([]byte, error)

// Resolver converts a namespace into YAML bytes by resolving the namespace
// to a git clone URL and file path, then fetching the file from the repository.
type Resolver struct {
	timeout time.Duration
	fetch   fetcher
}

// New returns a Resolver. If timeout is zero, a 30s default is used.
func New(
	timeout time.Duration,
) *Resolver {
	if timeout == 0 {
		timeout = defaultFetchTimeout
	}

	return &Resolver{
		timeout: timeout,
		fetch:   fetchFile,
	}
}

// ResolveArrow converts a namespace into the YAML bytes of an Arrow manifest.
func (r *Resolver) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, error) {
	cloneURL, filePath, err := resolveArrowParts(namespace)
	if err != nil {
		return nil, err
	}

	return r.fetch(ctx, cloneURL, filePath, r.timeout)
}

// ResolveQuiver converts a namespace into the YAML bytes of a Quiver manifest.
func (r *Resolver) ResolveQuiver(
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
	platform := parts[0]

	if _, ok := supportedPlatforms[platform]; !ok {
		return "", nil, ErrUnsupportedPlatform
	}

	cloneURL := fmt.Sprintf("https://%s/%s/%s", parts[0], parts[1], parts[2])

	return cloneURL, parts, nil
}
