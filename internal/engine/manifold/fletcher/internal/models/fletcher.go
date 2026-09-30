package models

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldModels "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

type Fletcher interface {
	// Recover drafts a manifest for a repository that ships none. Besides the
	// manifest and its filename it returns the ref the draft was built from,
	// which differs from ns's own when ns named a branch and the release
	// assets came from a tag.
	Recover(
		ctx context.Context,
		ns domain.Namespace,
		cause error,
	) ([]byte, string, string, error)
}

type Releases struct {
	LatestStable  func(ctx context.Context, ns domain.Namespace) (string, error)
	Channels      func(ctx context.Context, ns domain.Namespace) ([]manifoldModels.ChannelInfo, error)
	DefaultBranch func(ctx context.Context, ns domain.Namespace) (branch, hash string, err error)
}
