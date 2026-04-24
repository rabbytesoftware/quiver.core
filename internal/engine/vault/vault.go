package vault

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

const quiverFilename = "quiver.json"

type Vault interface {
	// GetArrow returns the cached raw manifest for the given namespace.
	// Returns ErrNotCached if no entry exists.
	// Returns ErrStale if TTL expired — ManifestFile is still returned.
	GetArrow(
		ctx context.Context,
		ns domain.Namespace,
	) (ManifestFile, error)

	GetQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverVaultEntry, string, error)

	// PutArrow also creates the namespace workdir as a side effect.
	PutArrow(
		ctx context.Context,
		ns domain.Namespace,
		file ManifestFile,
	) error

	PutQuiver(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.QuiverManifest,
	) (string, error)

	DeleteArrow(
		ctx context.Context,
		ns domain.Namespace,
	) error

	DeleteQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) error

	RenameArrow(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
	) error

	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]string, error)
}
