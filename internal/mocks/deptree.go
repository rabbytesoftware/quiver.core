package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
)

type MockDepTree struct {
	ResolveFunc deptree.DepTree
}

func (m *MockDepTree) Resolve(
	ctx context.Context,
	root domain.Namespace,
	resolver deptree.ResolverFunc,
) ([]domain.Namespace, error) {
	if m.ResolveFunc != nil {
		return m.ResolveFunc(ctx, root, resolver)
	}
	return nil, nil
}
