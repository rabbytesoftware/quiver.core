package resolver

import (
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// ErrUnsupportedPlatform is returned when a namespace domain is not one of
// the supported git hosting platforms.
var ErrUnsupportedPlatform = errors.New("resolver: unsupported git platform")

// ErrNotFound is returned when the manifest file does not exist in the
// remote repository.
var ErrNotFound = resolvers.ErrNotFound

// ErrFetchFailed is returned when the remote git repository cannot be cloned,
// HTTP request fails, or the file cannot be read from the in-memory worktree.
var ErrFetchFailed = resolvers.ErrFetchFailed
