package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

type Manifold struct {
	ResolveArrowResult      *domain.Arrow
	ResolveArrowRaw         []byte
	ResolveArrowFilename    string
	ResolveArrowErr         error
	ResolveArrowAtResult    *domain.Arrow
	ResolveArrowAtRaw       []byte
	ResolveArrowAtFilename  string
	ResolveArrowAtErr       error
	ResolveCollectionResult *domain.Collection
	ResolveCollectionErr    error
	ParseCollectionResult   *domain.Collection
	ParseCollectionErr      error
	ParseArrowResult        *domain.Arrow
	ParseArrowErr           error
	ListChannelsResult      []manifold.ChannelInfo
	ListChannelsErr         error
	SnapshotResult          domain.RefSnapshot
	SnapshotErr             error
	SnapshotCalls           int
	// FreshSnapshotCalls counts FreshSnapshot separately, since it answers
	// from the same SnapshotFn/SnapshotResult as Snapshot.
	FreshSnapshotCalls int
	// FreshSnapshotFn, when set, answers FreshSnapshot alone, so a test can
	// give the live remote a different view than the cached Snapshot.
	FreshSnapshotFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	ResolveArrowAtCommitResult   *domain.Arrow
	ResolveArrowAtCommitRaw      []byte
	ResolveArrowAtCommitFilename string
	ResolveArrowAtCommitErr      error

	// ResolveArrowCalls counts every ResolveArrow invocation, so a test can
	// assert a cache hit skipped the manifold entirely rather than only
	// checking the returned value.
	ResolveArrowCalls int

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

	// SnapshotFn, when set, answers per namespace so a test can move a ref
	// between calls.
	SnapshotFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	// ResolveArrowAtCommitFn, when set, answers per (namespace, commit) pair
	// so a test can assert which commit a caller fetched.
	ResolveArrowAtCommitFn func(
		ctx context.Context,
		ns domain.Namespace,
		commit string,
	) (*domain.Arrow, []byte, string, error)
}

func (m *Manifold) ResolveArrow(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	m.ResolveArrowCalls++
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

func (m *Manifold) ListChannels(
	_ context.Context,
	_ domain.Namespace,
) ([]manifold.ChannelInfo, error) {
	return m.ListChannelsResult, m.ListChannelsErr
}

func (m *Manifold) Snapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	m.SnapshotCalls++
	if m.SnapshotFn != nil {
		return m.SnapshotFn(ctx, ns)
	}
	return m.SnapshotResult, m.SnapshotErr
}

func (m *Manifold) FreshSnapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	m.FreshSnapshotCalls++
	if m.FreshSnapshotFn != nil {
		return m.FreshSnapshotFn(ctx, ns)
	}
	if m.SnapshotFn != nil {
		return m.SnapshotFn(ctx, ns)
	}
	return m.SnapshotResult, m.SnapshotErr
}

func (m *Manifold) ResolveArrowAtCommit(
	ctx context.Context,
	ns domain.Namespace,
	commit string,
) (*domain.Arrow, []byte, string, error) {
	if m.ResolveArrowAtCommitFn != nil {
		return m.ResolveArrowAtCommitFn(ctx, ns, commit)
	}
	return m.ResolveArrowAtCommitResult, m.ResolveArrowAtCommitRaw, m.ResolveArrowAtCommitFilename, m.ResolveArrowAtCommitErr
}
