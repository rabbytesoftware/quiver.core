package deptree

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ResolverFunc func(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error)

// DepTree resolves the full transitive dependency order for a given namespace.
type DepTree interface {
	Resolve(
		ctx context.Context,
		root domain.Namespace,
		resolver ResolverFunc,
	) ([]domain.Namespace, error)
}

type depTree struct{}

func New() DepTree {
	return &depTree{}
}

func (d *depTree) Resolve(
	ctx context.Context,
	root domain.Namespace,
	resolver ResolverFunc,
) ([]domain.Namespace, error) {
	t := NewTraversal(ctx, resolver)

	if err := t.dfs(root); err != nil {
		return nil, err
	}

	return t.order, nil
}
