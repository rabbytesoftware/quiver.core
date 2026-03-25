package deptree

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ResolverFunc func(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error)

type DepTree func(
	ctx context.Context,
	root domain.Namespace,
	resolver ResolverFunc,
) ([]domain.Namespace, error)

func Deptree(
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
