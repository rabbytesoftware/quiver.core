package client

import "context"

type QuiverClient interface {
	ArrowAdd(ctx context.Context, ns string) error
	ArrowUpdate(ctx context.Context, ns string) error
	ArrowRemove(ctx context.Context, ns string) error
	ArrowList(ctx context.Context, userInstalled bool) ([]ArrowListItem, error)
	ArrowGet(ctx context.Context, ns string) (*ArrowDetail, error)
	ArrowGetManifest(ctx context.Context, ns string) ([]byte, error)
	ArrowSeed(ctx context.Context, ns string, manifest []byte) error
	ArrowValidate(ctx context.Context, ns string, manifest []byte) (*ValidationResult, error)

	Install(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	Update(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	Execute(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	Stop(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	Uninstall(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	RunMethod(ctx context.Context, ns, method string, vars map[string]string) (<-chan ArrowRuntime, error)

	RuntimeGet(ctx context.Context, ns string) (*ArrowRuntime, error)
	RuntimeList(ctx context.Context) ([]ArrowRuntime, error)
	WatchRuntime(ctx context.Context, ns string) (<-chan ArrowRuntime, error)

	CollectionAdd(ctx context.Context, ns string) error
	CollectionUpdate(ctx context.Context, ns string) error
	CollectionRemove(ctx context.Context, ns string) error
	CollectionList(ctx context.Context) ([]Collection, error)
	CollectionGet(ctx context.Context, ns string) (*Collection, error)

	Health(ctx context.Context) (*HealthStatus, error)
}
