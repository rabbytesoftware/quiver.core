package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
)

// DepTree returns a DepTree func that always returns the provided result.
func DepTree(result []domain.Namespace, err error) deptree.DepTree {
	return func(_ context.Context, _ domain.Namespace, _ deptree.ResolverFunc) ([]domain.Namespace, error) {
		return result, err
	}
}
