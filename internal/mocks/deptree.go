package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
)

type DepTree struct {
	Result []domain.Namespace
	Err    error
}

func (m *DepTree) Resolve(
	_ context.Context,
	_ domain.Namespace,
	_ deptree.ResolverFunc,
) ([]domain.Namespace, error) {
	return m.Result, m.Err
}
