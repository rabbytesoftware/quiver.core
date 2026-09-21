package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Manifold struct {
	ResolveArrowResult        *domain.Arrow
	ResolveArrowRaw           []byte
	ResolveArrowFilename      string
	ResolveArrowErr           error
	ResolveArrowAtResult      *domain.Arrow
	ResolveArrowAtRaw         []byte
	ResolveArrowAtFilename    string
	ResolveArrowAtErr         error
	ResolveCollectionResult   *domain.Collection
	ResolveCollectionErr      error
	ParseCollectionResult     *domain.Collection
	ParseCollectionErr        error
	ParseArrowResult          *domain.Arrow
	ParseArrowErr             error
	ResolveConstraintResult   string
	ResolveConstraintErr      error
	ResolveLatestStableRef    string
	ResolveLatestStableErr    error
	DefaultBranchRef          string
	DefaultBranchHash         string
	DefaultBranchErr          error
	ResolveLatestInChannelRef string
	ResolveLatestInChannelErr error

	// ResolveArrowFunc, when set, answers per namespace so a test can make one
	// ref resolve while another misses.
	ResolveArrowFunc func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, []byte, string, error)

	// ResolveArrowAtFunc, when set, answers per (namespace, path) pair so a
	// test can capture or vary the path a caller supplies.
	ResolveArrowAtFunc func(
		ctx context.Context,
		ns domain.Namespace,
		path string,
	) (*domain.Arrow, []byte, string, error)
}

func (m *Manifold) ResolveArrow(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	if m.ResolveArrowFunc != nil {
		return m.ResolveArrowFunc(ctx, ns)
	}
	return m.ResolveArrowResult, m.ResolveArrowRaw, m.ResolveArrowFilename, m.ResolveArrowErr
}

func (m *Manifold) ResolveArrowAt(
	ctx context.Context,
	ns domain.Namespace,
	path string,
) (*domain.Arrow, []byte, string, error) {
	if m.ResolveArrowAtFunc != nil {
		return m.ResolveArrowAtFunc(ctx, ns, path)
	}
	return m.ResolveArrowAtResult, m.ResolveArrowAtRaw, m.ResolveArrowAtFilename, m.ResolveArrowAtErr
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

func (m *Manifold) ResolveDefaultBranch(
	_ context.Context,
	_ domain.Namespace,
) (string, string, error) {
	return m.DefaultBranchRef, m.DefaultBranchHash, m.DefaultBranchErr
}

func (m *Manifold) ResolveLatestInChannel(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return m.ResolveLatestInChannelRef, m.ResolveLatestInChannelErr
}
