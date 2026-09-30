package advance_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/advance"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func rollingNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@nightly-latest")
}

func seedSelectorRow(
	t *testing.T,
	axArrow asynx.Asynx[domain.Arrow],
	ns domain.Namespace,
	kind domain.SelectorKind,
	resolved domain.Resolved,
) {
	t.Helper()
	_, err := axArrow.SendWait(context.Background(), arrowcmds.AddArrow{
		Namespace:     ns,
		ArrowMeta:     domain.ArrowMeta{Name: "Old"},
		DirectInstall: true,
		SelectorKind:  kind,
		Resolved:      resolved,
	})
	require.NoError(t, err)
}

// failOnNetworkManifold answers ParseArrow and fails the test on any remote
// fetch, so a test proves a method never touched the network.
func failOnNetworkManifold(t *testing.T, parsed *domain.Arrow) *mocks.Manifold {
	t.Helper()
	return &mocks.Manifold{
		ParseArrowResult: parsed,
		ResolveArrowFunc: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error) {
			t.Errorf("ResolveArrow(%s) must never be called", ns)
			return nil, nil, "", errors.New("network")
		},
		ResolveArrowAtCommitFn: func(_ context.Context, ns domain.Namespace, _, _ string) (*domain.Arrow, []byte, string, error) {
			t.Errorf("ResolveArrowAtCommit(%s) must never be called", ns)
			return nil, nil, "", errors.New("network")
		},
	}
}

// adoptedManifest is a parsed manifest with every field RefreshManifest
// carries set, so a comparison that skipped one would show.
func adoptedManifest(name string) *domain.Arrow {
	return &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: name, Description: name + " description"},
		Variables: []domain.Variable{{Name: name + "_VAR", Default: "1"}},
		Targets:   map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
		Readme:    name + " readme",
	}
}

func stableNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@stable")
}

func stableSnapshot(latestTag, commit string) domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags:     map[string]string{"v1.0.0": "c100", latestTag: commit},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
}

const adoptBare = domain.Namespace("github.com/rabbytesoftware/quiver.desktop")

// conflictingAsynx is a real arrow aggregate whose next SendWait calls lose a
// version-conflict race, the way a concurrent append to the row makes them.
type conflictingAsynx struct {
	asynx.Asynx[domain.Arrow]
	conflicts int32
	sends     atomic.Int32
}

func (c *conflictingAsynx) SendWait(
	ctx context.Context,
	cmd asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	if c.sends.Add(1) <= c.conflicts {
		return asynxModels.Event[domain.Arrow]{}, versionConflict()
	}
	return c.Asynx.SendWait(ctx, cmd)
}

func targetManifold() *mocks.Manifold {
	return &mocks.Manifold{
		ResolveArrowAtCommitFn: func(_ context.Context, ns domain.Namespace, _, _ string) (*domain.Arrow, []byte, string, error) {
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Target"}}, []byte("raw"), "arrow.yaml", nil
		},
	}
}

// versionConflict is what asynx returns when another writer appended to the
// row between this writer's read and its append.
func versionConflict() error {
	return fmt.Errorf("%w: version conflict", asynxModels.ErrPipelineFailed)
}

// rowSequence answers Get with rows in turn, repeating the last one.
func rowSequence(rows ...domain.Arrow) func(context.Context, string) (domain.Arrow, error) {
	var calls atomic.Int32
	return func(context.Context, string) (domain.Arrow, error) {
		i := int(calls.Add(1)) - 1
		if i >= len(rows) {
			i = len(rows) - 1
		}
		return rows[i], nil
	}
}

type testOpts struct {
	outdated func(ctx context.Context, ns domain.Namespace, outdated bool) error
}

type testOption func(*testOpts)

// withVersionOutdatedSync stands in for the arrow repository's own sync of
// the runtime badge, which reads the row and pushes whether it is outdated.
func withVersionOutdatedSync(
	fn func(ctx context.Context, ns domain.Namespace, outdated bool) error,
) testOption {
	return func(o *testOpts) { o.outdated = fn }
}

func newTestable(
	r arrowstore.Store,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	opts ...testOption,
) advance.Advancer {
	var o testOpts
	for _, apply := range opts {
		apply(&o)
	}
	return advance.New(r, axArrow, v, m, func(ctx context.Context, ns domain.Namespace) {
		if o.outdated == nil {
			return
		}
		row, err := axArrow.Get(ctx, ns.String())
		if err != nil {
			return
		}
		_ = o.outdated(ctx, ns, row.Available != nil)
	})
}
