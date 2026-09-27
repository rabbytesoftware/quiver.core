package hosts

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Forge interface {
	ReleaseAssets(
		ctx context.Context,
		ns domain.Namespace,
		tag string,
	) ([]Asset, error)

	RepoPage(
		ctx context.Context,
		ns domain.Namespace,
	) (RepoPage, error)

	RawFile(
		ctx context.Context,
		ns domain.Namespace,
		ref string,
		path string,
	) ([]byte, error)
}
