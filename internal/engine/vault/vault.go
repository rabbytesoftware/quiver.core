package vault

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

const (
	arrowFilename  = "arrow.json"
	quiverFilename = "quiver.json"
)

type Vault interface {
	// GetArrow returns the cached entry for the given namespace.
	// Returns ErrNotCached if no entry exists.
	// Returns ErrStale if TTL expired — entry and path are still returned.
	GetArrow(
		ctx context.Context,
		ns domain.Namespace,
	) (*VaultEntry, string, error)

	// GetQuiver returns the cached entry for the given namespace.
	// Same error semantics as GetArrow.
	GetQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverVaultEntry, string, error)

	// PutArrow persists the manifest for the given namespace and returns the home directory path.
	// indirectDeps may be nil (pre-install) or populated (post-install, after DepTree runs).
	PutArrow(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.ArrowManifest,
		indirectDeps []domain.Namespace,
	) (string, error)

	// PutQuiver persists the manifest for the given namespace and returns the home directory path.
	PutQuiver(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.QuiverManifest,
	) (string, error)

	// DeleteArrow removes arrow.json. If quiver.json is absent too, removes the whole home directory.
	// Idempotent — returns nil if the entry does not exist.
	DeleteArrow(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// DeleteQuiver removes quiver.json. If arrow.json is absent too, removes the whole home directory.
	// Idempotent — returns nil if the entry does not exist.
	DeleteQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// ListVersions returns the ref strings of all cached versions for the given bare namespace.
	// Non-existent namespace and namespaces with no cached versions both return an empty slice.
	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]string, error)
}
