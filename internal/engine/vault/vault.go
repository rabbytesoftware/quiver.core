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

	// GetQuiver returns the cached entry for the given namespace.
	// Same error semantics as GetArrow.
	GetQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) (*QuiverVaultEntry, string, error)

	// PutArrow persists the raw manifest file for the given namespace.
	// Also creates the namespace workdir as a side effect.
	PutArrow(
		ctx context.Context,
		ns domain.Namespace,
		file ManifestFile,
	) error

	// PutQuiver persists the manifest for the given namespace and returns the home directory path.
	PutQuiver(
		ctx context.Context,
		ns domain.Namespace,
		manifest *domain.QuiverManifest,
	) (string, error)

	// DeleteArrow removes the manifest file and its meta sidecar. Idempotent.
	DeleteArrow(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// DeleteQuiver removes quiver.json. Idempotent.
	DeleteQuiver(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// RenameArrow moves the cached arrow entry from oldNs to newNs.
	// Idempotent if oldNs == newNs.
	RenameArrow(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
	) error

	// ListVersions returns the ref strings of all cached versions for the given bare namespace.
	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]string, error)
}
