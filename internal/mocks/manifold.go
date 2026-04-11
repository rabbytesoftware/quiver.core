package mocks

import (
	"context"
	"errors"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Manifold struct {
	ResolveArrowManifest  *domain.ArrowManifest
	ResolveArrowErr       error
	ResolveQuiverManifest *domain.QuiverManifest
	ResolveQuiverErr      error
	ParseArrowManifest    *domain.ArrowManifest
	ParseArrowErr         error
}

func (m *Manifold) ResolveArrow(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
	return m.ResolveArrowManifest, m.ResolveArrowErr
}

func (m *Manifold) ParseArrow(_ []byte) (*domain.ArrowManifest, error) {
	return m.ParseArrowManifest, m.ParseArrowErr
}

func (m *Manifold) ResolveQuiver(_ context.Context, _ domain.Namespace) (*domain.QuiverManifest, error) {
	if m.ResolveQuiverManifest != nil || m.ResolveQuiverErr != nil {
		return m.ResolveQuiverManifest, m.ResolveQuiverErr
	}
	return nil, errors.New("not implemented")
}
