package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Manifold struct {
	ResolveArrowResult      *domain.Arrow
	ResolveArrowRaw         []byte
	ResolveArrowFilename    string
	ResolveArrowErr         error
	ResolveQuiverResult     *domain.Quiver
	ResolveQuiverErr        error
	ParseQuiverResult       *domain.Quiver
	ParseQuiverErr          error
	ParseArrowResult        *domain.Arrow
	ParseArrowErr           error
	ResolveConstraintResult string
	ResolveConstraintErr    error
}

func (m *Manifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	return m.ResolveArrowResult, m.ResolveArrowRaw, m.ResolveArrowFilename, m.ResolveArrowErr
}

func (m *Manifold) ParseQuiver(
	_ []byte,
	_ domain.Namespace,
) (*domain.Quiver, error) {
	return m.ParseQuiverResult, m.ParseQuiverErr
}

func (m *Manifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return m.ParseArrowResult, m.ParseArrowErr
}

func (m *Manifold) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Quiver, error) {
	return m.ResolveQuiverResult, m.ResolveQuiverErr
}

func (m *Manifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return m.ResolveConstraintResult, m.ResolveConstraintErr
}
