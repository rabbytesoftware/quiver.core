package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Manifold struct {
	ResolveArrowManifest    *domain.ArrowManifest
	ResolveArrowErr         error
	ResolveQuiverManifest   *domain.QuiverManifest
	ResolveQuiverErr        error
	ParseArrowManifest      *domain.ArrowManifest
	ParseArrowErr           error
	ResolveConstraintResult string
	ResolveConstraintErr    error
}

func (m *Manifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.ArrowManifest, error) {
	return m.ResolveArrowManifest, m.ResolveArrowErr
}

func (m *Manifold) ParseArrow(
	_ []byte,
) (*domain.ArrowManifest, error) {
	return m.ParseArrowManifest, m.ParseArrowErr
}

func (m *Manifold) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*domain.QuiverManifest, error) {
	return m.ResolveQuiverManifest, m.ResolveQuiverErr
}

func (m *Manifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return m.ResolveConstraintResult, m.ResolveConstraintErr
}
