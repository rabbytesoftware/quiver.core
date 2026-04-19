package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
)

type StubStore struct {
	HasDependents bool
	DependentsErr error
}

func (s *StubStore) HasAnyDependents(
	_ context.Context,
	_, _ string,
) (bool, error) {
	return s.HasDependents, s.DependentsErr
}

func (s *StubStore) ByDependency(
	_ context.Context,
	_, _ string,
) ([]store.DepEdgeRow, error) {
	return nil, nil
}
