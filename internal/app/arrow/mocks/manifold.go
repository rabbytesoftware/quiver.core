package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Manifold struct {
	ResolveArrowValue    *domain.Arrow
	ResolveArrowRaw      []byte
	ResolveArrowFilename string
	ResolveArrowErr      error
	ParseArrowValue      *domain.Arrow
	ParseArrowErr        error
	ConstraintValue      string
	ConstraintErr        error
}

func (m *Manifold) ResolveArrow(ctx context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error) {
	return m.ResolveArrowValue, m.ResolveArrowRaw, m.ResolveArrowFilename, m.ResolveArrowErr
}

func (m *Manifold) ResolveQuiver(ctx context.Context, ns domain.Namespace) (*domain.QuiverManifest, error) {
	return nil, nil
}

func (m *Manifold) ParseArrow(data []byte) (*domain.Arrow, error) {
	return m.ParseArrowValue, m.ParseArrowErr
}

func (m *Manifold) ResolveConstraint(ctx context.Context, ns domain.Namespace, pattern string) (string, error) {
	return m.ConstraintValue, m.ConstraintErr
}
