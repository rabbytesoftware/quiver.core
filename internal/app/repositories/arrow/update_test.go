package arrow_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func stableNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@stable")
}

// ─── CheckAvailable ──────────────────────────────────────────────────────────

// ─── TargetUnmoved ───────────────────────────────────────────────────────────

// ─── RefreshToTarget ─────────────────────────────────────────────────────────

func TestRefreshToTarget_StagesTheTargetManifestOnTheSameRow(t *testing.T) {
	ctx := context.Background()
	ns := rollingNs()
	axArrow := newTestAsynxArrow(t)

	var mu sync.Mutex
	var projected []domain.Arrow
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, a domain.Arrow) error {
			mu.Lock()
			defer mu.Unlock()
			projected = append(projected, a)
			return nil
		},
	}
	v := &mocks.Vault{}
	var fetchedRef, fetchedCommit string
	m := &mocks.Manifold{
		ResolveArrowAtCommitFn: func(_ context.Context, got domain.Namespace, ref, commit string) (*domain.Arrow, []byte, string, error) {
			fetchedRef, fetchedCommit = ref, commit
			return &domain.Arrow{Namespace: got, ArrowMeta: domain.ArrowMeta{Name: "Target"}}, []byte("raw"), "arrow.yaml", nil
		},
	}
	hub := &recordingHub{}
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, v, m, hub)
	require.NoError(t, err)
	var updated atomic.Int32
	require.NoError(t, cat.OnArrowUpdated(func(context.Context, domain.Namespace, *domain.Arrow) error {
		updated.Add(1)
		return nil
	}))

	installed := domain.Resolved{Ref: "nightly-latest", Commit: "c1", Fingerprint: "c1"}
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, installed)
	_, err = axArrow.SendWait(ctx, arrowcmds.RecordAvailable{Namespace: ns, Available: &target, JudgedResolved: installed})
	require.NoError(t, err)

	staged, err := cat.RefreshToTarget(ctx, ns, target)

	require.NoError(t, err)
	assert.Equal(t, "Target", staged.Name)
	assert.Equal(t, ns, staged.Namespace)
	assert.Equal(t, "c2", fetchedCommit)
	assert.Equal(t, "nightly-latest", fetchedRef)

	row, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Target", row.Name)
	assert.Equal(t, installed, row.Resolved, "nothing is installed until the update commits")
	assert.Equal(t, &target, row.Available)

	assert.Equal(t, []string{"delete " + ns.String(), "put " + ns.String()}, v.ArrowOps)
	require.NotEmpty(t, v.PutArrowFiles)
	assert.Equal(t, "arrow.yaml", v.PutArrowFiles[len(v.PutArrowFiles)-1].Filename)

	assert.Equal(t, int32(1), updated.Load(), "a refreshed manifest re-syncs the dependency graph")
	mu.Lock()
	last := projected[len(projected)-1]
	mu.Unlock()
	assert.Equal(t, "Target", last.Name, "the read model carries the staged manifest")
	assert.Equal(t, installed, last.Resolved)
	assert.Equal(t, apphub.CatalogUpserted, hub.kinds()[len(hub.kinds())-1])
}

// ─── AddDependency ───────────────────────────────────────────────────────────
