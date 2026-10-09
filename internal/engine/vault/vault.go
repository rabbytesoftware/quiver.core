package vault

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const quiverFilename = "collection.json"

type Vault interface {
	// GetArrow returns the cached raw manifest for the given namespace.
	// Returns ErrNotCached if no entry exists.
	// Returns ErrStale if TTL expired — ManifestFile is still returned.
	// Returns ErrConfirmedAbsent if a fresh PutArrowNotFound marker exists —
	// no ManifestFile content, since there was never any to cache, only the
	// commit the marker was recorded for.
	GetArrow(
		ctx context.Context,
		ns domain.Namespace,
	) (ManifestFile, error)

	GetCollection(
		ctx context.Context,
		ns domain.Namespace,
	) (*CollectionVaultEntry, string, error)

	// GetRefs returns the ref list last saved for ns's repository, whatever
	// its age. Returns ErrNotCached if none was saved or the file is unreadable.
	GetRefs(
		ctx context.Context,
		ns domain.Namespace,
	) (RefsEntry, error)

	// PutRefs saves snap as the ref list of ns's repository, stamped now.
	PutRefs(
		ctx context.Context,
		ns domain.Namespace,
		snap domain.RefSnapshot,
	) error

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

	// PutArrowNotFound records that ns's manifest was resolved live and
	// definitively does not exist, so a later GetArrow reports
	// ErrConfirmedAbsent instead of ErrNotCached until the same TTL as a
	// positive entry expires. Callers must only call this for a genuine
	// not-found (the fetch reached the repository and inspected its tree),
	// never for a transient failure — caching a network blip as "absent"
	// would convert a temporary outage into a false not-found for a full TTL.
	// commit is the commit the fetch looked at, empty when unknown; GetArrow
	// reports it on the ManifestFile it returns with ErrConfirmedAbsent.
	PutArrowNotFound(
		ctx context.Context,
		ns domain.Namespace,
		commit string,
	) error

	PutCollection(
		ctx context.Context,
		ns domain.Namespace,
		quiver *domain.Collection,
	) (string, error)

	ListCachedCollections(ctx context.Context) ([]domain.Namespace, error)

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

	DeleteCollection(
		ctx context.Context,
		ns domain.Namespace,
	) error

	ListVersions(
		ctx context.Context,
		ns domain.Namespace,
	) ([]string, error)

	// SearchArrows queries the read model built from every manifest the vault
	// has cached. Rows outlive the manifest bytes they were built from, so a
	// hit does not imply GetArrow will succeed.
	SearchArrows(
		ctx context.Context,
		q IndexQuery,
	) ([]IndexRow, error)

	// ForgetArrow drops every indexed ref under a namespace. The cached
	// manifest files are left alone.
	ForgetArrow(
		ctx context.Context,
		ns domain.Namespace,
	) error

	// Start launches the periodic manifest sweep goroutine.
	// Sweeps run on the interval set by vault.sweep_interval in config.yaml (default 5m).
	// The goroutine exits when ctx is cancelled.
	Start(ctx context.Context)

	// Close releases the index database opened by New. It is the counterpart the
	// constructor owes its handle, and it makes Vault an io.Closer so a failed
	// construction can discard it with the rest.
	//
	// Every method that reads or writes the index reports ErrClosed afterwards.
	// Close is idempotent.
	Close() error
}
