package models

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Fletcher interface {
	Recover(
		ctx context.Context,
		ns domain.Namespace,
		cause error,
	) ([]byte, string, error)
}

type Releases interface {
	LatestStable(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

	LatestUnstable(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

	ResolveDefaultBranch(
		ctx context.Context,
		ns domain.Namespace,
	) (string, string, error)
}
