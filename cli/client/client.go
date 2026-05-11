package client

import "context"

// QuiverClient is the boundary between cmd/ and the HTTP+WS transport layer.
// cmd/ depends only on this interface — never on HTTPClient directly.
type QuiverClient interface {
	// Arrow catalog
	ArrowList(ctx context.Context, userInstalled bool) ([]ArrowListItem, error)
	ArrowGet(ctx context.Context, ns string) (*ArrowDetail, error)
	ArrowGetManifest(ctx context.Context, ns string) ([]byte, error)
	ArrowAdd(ctx context.Context, ns string) error
	ArrowUpdate(ctx context.Context, ns string) error
	ArrowRemove(ctx context.Context, ns string) error
	ArrowSeed(ctx context.Context, ns string, manifest []byte) error
	ArrowValidate(ctx context.Context, ns string, manifest []byte) (*ValidationResult, error)

	// Runtime lifecycle — fires POST /v0/runtime/:ns/:method then streams ArrowRuntime
	// snapshots over WS. The channel is closed when the operation reaches its terminal state.
	Install(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	Uninstall(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	Run(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	Stop(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	Update(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	RunMethod(ctx context.Context, ns, method string, vars map[string]string) (<-chan ArrowRuntime, error)

	// Runtime observation — streams ArrowRuntime snapshots.
	// RuntimeGet and RuntimeList dial WS, read one snapshot, then close (no REST GET exists).
	// WatchRuntime keeps the channel open until ctx is cancelled.
	RuntimeGet(ctx context.Context, ns string) (*ArrowRuntime, error)
	RuntimeList(ctx context.Context) ([]ArrowRuntime, error)
	WatchRuntime(ctx context.Context, ns string) (<-chan ArrowRuntime, error)

	// Collections
	CollectionList(ctx context.Context) ([]Collection, error)
	CollectionGet(ctx context.Context, ns string) (*Collection, error)
	CollectionAdd(ctx context.Context, ns string) error
	CollectionUpdate(ctx context.Context, ns string) error
	CollectionRemove(ctx context.Context, ns string) error

	// System
	Health(ctx context.Context) (*HealthStatus, error)
}
