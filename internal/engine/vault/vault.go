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

	// WorkDir returns the namespace workdir path, creating it on disk if it
	// does not already exist. Both arrows and quivers share the same workdir
	// layout under namespacesPath. Callers should not construct or create
	// workdir paths themselves — use this method instead.
	WorkDir(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

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

	// DeleteWorkDir removes the namespace workdir tree from disk.
	// The vault cache (manifest + meta files) is left intact.
	// Called on arrow/quiver removal so temporary build artifacts are cleaned up.
	DeleteWorkDir(
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

	// Start launches the periodic manifest sweep goroutine.
	// Sweeps run on the interval set by vault.sweep_interval in config.yaml (default 5m).
	// The goroutine exits when ctx is cancelled.
	Start(ctx context.Context)
}
