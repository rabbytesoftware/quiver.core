package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Manifold struct {
	ResolveArrowResult      *domain.Arrow
	ResolveArrowRaw         []byte
	ResolveArrowFilename    string
	ResolveArrowErr         error
	ResolveCollectionResult *domain.Collection
	ResolveCollectionErr    error
	ParseCollectionResult   *domain.Collection
	ParseCollectionErr      error
	ParseArrowResult        *domain.Arrow
	ParseArrowErr           error
	ResolveConstraintResult string
	ResolveConstraintErr    error
	ResolveLatestStableRef  string
	ResolveLatestStableErr  error
}

func (m *Manifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	return m.ResolveArrowResult, m.ResolveArrowRaw, m.ResolveArrowFilename, m.ResolveArrowErr
}

func (m *Manifold) ParseCollection(
	_ []byte,
	_ domain.Namespace,
) (*domain.Collection, error) {
	return m.ParseCollectionResult, m.ParseCollectionErr
}

func (m *Manifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return m.ParseArrowResult, m.ParseArrowErr
}

func (m *Manifold) ResolveCollection(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Collection, error) {
	return m.ResolveCollectionResult, m.ResolveCollectionErr
}

func (m *Manifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return m.ResolveConstraintResult, m.ResolveConstraintErr
}

func (m *Manifold) ResolveLatestStable(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return m.ResolveLatestStableRef, m.ResolveLatestStableErr
}
