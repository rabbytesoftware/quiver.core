package arrow_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	arrowMocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	runtimeRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

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

// newProjectingTestable builds a catalog with its subscribers registered, so a
// test can drive real events through the single projection that runs the
// callbacks.
func newProjectingTestable(
	t *testing.T,
	r *arrowStoreMocks.MockCQRS,
	axArrow asynx.Asynx[domain.Arrow],
) arrowRepo.Arrow {
	t.Helper()
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, nil, nil, nil)
	require.NoError(t, err)
	return cat
}

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func testArrow() *domain.Arrow {
	return &domain.Arrow{
		Namespace: testNs(),
		ArrowMeta: domain.ArrowMeta{Name: "Test Arrow"},
	}
}

// ─── Tests ───────────────────────────────────────────────────────────────────

func TestList_DelegatesToCQRS(t *testing.T) {
	view := models.ArrowView{
		Namespace: testNs().BareNamespace(),
		Metadata:  *testArrow(),
		Versions:  []models.VersionView{{Namespace: testNs(), State: domain.ArrowStateReady}},
	}
	r := &arrowStoreMocks.MockCQRS{
		ListFn: func(ctx context.Context, userInstalled *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{view}, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	result, err := cat.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, testNs().BareNamespace(), result[0].Namespace)
}

func TestList_Error(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ListFn: func(ctx context.Context, userInstalled *bool) ([]models.ArrowView, error) {
			return nil, errors.New("db error")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	_, err := cat.List(context.Background(), nil)
	require.Error(t, err)
}

func TestGet_DelegatesToCQRS(t *testing.T) {
	expected := testArrow()
	r := &arrowStoreMocks.MockCQRS{
		GetFn: func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return expected, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	got, err := cat.Get(context.Background(), testNs())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestExists_UsesAsynxArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)

	// Arrow does not exist yet
	exists, err := cat.Exists(context.Background(), testNs())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestGetDetail_DelegatesToCQRS(t *testing.T) {
	expected := &models.ArrowDetailView{
		Metadata: *testArrow(),
		State:    domain.ArrowStateReady,
	}
	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(ctx context.Context, ns domain.Namespace) (*models.ArrowDetailView, error) {
			return expected, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	got, err := cat.GetDetail(context.Background(), testNs())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

// ─── GetDetail: version-check trigger ────────────────────────────────────────

func TestGetDetail_NilView_SkipsVersionCheck(t *testing.T) {
	var needsCalled atomic.Bool
	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(context.Context, domain.Namespace) (*models.ArrowDetailView, error) {
			return nil, nil
		},
		NeedsVersionCheckFn: func(context.Context, domain.Namespace, time.Time) (bool, error) {
			needsCalled.Store(true)
			return false, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	view, err := cat.GetDetail(context.Background(), testNs())
	require.NoError(t, err)
	assert.Nil(t, view)
	assert.False(t, needsCalled.Load())
}

func TestGetDetail_StoreError_PropagatesAndSkipsCheck(t *testing.T) {
	wantErr := errors.New("boom")
	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(context.Context, domain.Namespace) (*models.ArrowDetailView, error) {
			return nil, wantErr
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	_, err := cat.GetDetail(context.Background(), testNs())
	assert.ErrorIs(t, err, wantErr)
}

func TestGetDetail_DoesNotNeedCheck_NeverCallsDrift(t *testing.T) {
	ns := testNs()
	var driftCalled atomic.Bool
	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(context.Context, domain.Namespace) (*models.ArrowDetailView, error) {
			return &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: ns}}, nil
		},
		NeedsVersionCheckFn: func(context.Context, domain.Namespace, time.Time) (bool, error) {
			return false, nil
		},
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			driftCalled.Store(true)
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	_, err := cat.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.False(t, driftCalled.Load())
}

// NeedsVersionCheck erroring must not fail the read GetDetail exists to
// serve — the version signal is best-effort, the detail view is not.
func TestGetDetail_NeedsVersionCheckErrors_StillReturnsView(t *testing.T) {
	ns := testNs()
	var driftCalled atomic.Bool
	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(context.Context, domain.Namespace) (*models.ArrowDetailView, error) {
			return &models.ArrowDetailView{Metadata: domain.Arrow{Namespace: ns}}, nil
		},
		NeedsVersionCheckFn: func(context.Context, domain.Namespace, time.Time) (bool, error) {
			return false, errors.New("db down")
		},
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			driftCalled.Store(true)
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	view, err := cat.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.False(t, driftCalled.Load())
}

// The end-to-end path: GetDetail launches the check in the background (the
// call itself returns immediately) and the check's outcome lands on the
// aggregate once the goroutine runs. This is the test that would catch the
// whole feature being silently inert at the trigger layer, the mirror of
// TestProjectVersionChecked_WritesReadModelAndAnnounces at the projection
// layer.
func TestGetDetail_NeedsCheck_LaunchesBackgroundCheckThatLands(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		GetDetailFn: func(context.Context, domain.Namespace) (*models.ArrowDetailView, error) {
			return &models.ArrowDetailView{Metadata: arrow}, nil
		},
		NeedsVersionCheckFn: func(context.Context, domain.Namespace, time.Time) (bool, error) {
			return true, nil
		},
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	_, err = cat.GetDetail(context.Background(), ns)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, getErr := axArrow.Get(context.Background(), ns.String())
		return getErr == nil && got.Outdated && got.RecommendedRef == "v2.0.0"
	}, 2*time.Second, 10*time.Millisecond, "background version check never landed on the aggregate")
}

// ─── runVersionCheck ─────────────────────────────────────────────────────────

func TestRunVersionCheck_DiffFound_SendsRecordVersionCheck(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.Outdated)
	assert.Equal(t, "v2.0.0", got.RecommendedRef)
}

// A check that reconfirms the aggregate's existing answer must send nothing —
// verified by subscribing to the topic itself, not by inference from state.
func TestRunVersionCheck_NoDiff_SendsNoCommand(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)

	var fired atomic.Int32
	_, err = axArrow.Subscribe(asynx.Topic("arrow.version_checked.*"), func(
		context.Context,
		asynxModels.Event[domain.Arrow],
	) {
		fired.Add(1)
	})
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return false, "", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)
	axArrow.WaitPublish()

	assert.Equal(t, int32(0), fired.Load())
}

// ok=false must abort before ever touching the aggregate — a resolution
// failure is never a trustworthy "not outdated" either.
func TestRunVersionCheck_ResolutionFailed_AbortsSilently(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)
	arrow, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return false, "", false
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), arrow)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.Outdated)
	assert.Empty(t, got.RecommendedRef)
}

// A namespace the aggregate never existed for (e.g. removed between the claim
// and the check running) must not panic axArrow.Get's error away.
func TestRunVersionCheck_AggregateGone_AbortsSilently(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	r := &arrowStoreMocks.MockCQRS{
		CheckVersionDriftFn: func(context.Context, domain.Arrow) (bool, string, bool) {
			return true, "v2.0.0", true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)

	assert.NotPanics(t, func() {
		arrowRepo.RunVersionCheckForTest(cat, context.Background(), domain.Arrow{Namespace: ns})
	})
}

func TestGetManifest_DelegatesToCQRS(t *testing.T) {
	expected := testArrow()
	r := &arrowStoreMocks.MockCQRS{
		GetManifestFn: func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return expected, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	got, err := cat.GetManifest(context.Background(), testNs())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestResolveManifest_DelegatesToCQRS(t *testing.T) {
	expected := testArrow()
	r := &arrowStoreMocks.MockCQRS{
		ResolveManifestFn: func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return expected, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	got, err := cat.ResolveManifest(context.Background(), testNs())
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestResolveForInstall_DelegatesToCQRS(t *testing.T) {
	expected := testArrow()
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, ns domain.Namespace, channel string) (domain.Namespace, *domain.Arrow, string, error) {
			return testNs(), expected, "^v1", nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	resolvedNs, arrow, constraint, err := cat.ResolveForInstall(context.Background(), testNs(), "")
	require.NoError(t, err)
	assert.Equal(t, testNs(), resolvedNs)
	assert.Equal(t, expected, arrow)
	assert.Equal(t, "^v1", constraint)
}

func TestMarkInstalled_SendsCommand(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Seed an arrow first so MarkInstalled can find it
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.MarkInstalled(context.Background(), ns, time.Now().UTC())
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.InstalledAt.IsZero())
	assert.Equal(t, "v1.0.0", got.Namespace.Ref())
}

// A namespace that resolved to a default branch is installed at that branch: the
// branch is the ref, so the stamp lands on the branch's own aggregate.
func TestMarkInstalled_DefaultBranchRef_StampsTheBranchRow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := domain.Namespace("github.com/user/repo@master")

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.MarkInstalled(context.Background(), ns, time.Now().UTC())
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.InstalledAt.IsZero())
	assert.Equal(t, "master", got.Namespace.Ref())
}

func TestMarkUninstalled_ClearsTheInstalledStamp(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	require.NoError(t, cat.MarkInstalled(context.Background(), ns, time.Now().UTC()))
	require.NoError(t, cat.MarkUninstalled(context.Background(), ns))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.InstalledAt.IsZero())
}

// Nothing installs an arrow that is not in the catalog, so a stamp asked for on
// an unknown namespace is a state violation rather than a silent no-op.
func TestMarkUninstalled_UnknownNamespace_Errors(t *testing.T) {
	axArrow := newTestAsynxArrow(t)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.MarkUninstalled(context.Background(), testNs())

	require.Error(t, err)
}

func TestMarkLastUsed_SendsCommand(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Seed an arrow first so MarkLastUsed can find it
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.MarkLastUsed(context.Background(), ns, time.Now().UTC())
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.LastUsedAt.IsZero())
	assert.Equal(t, "v1.0.0", got.Namespace.Ref())
}

// Nothing runs an arrow that is not in the catalog, so a stamp asked for on
// an unknown namespace is a state violation rather than a silent no-op.
func TestMarkLastUsed_UnknownNamespace_Errors(t *testing.T) {
	axArrow := newTestAsynxArrow(t)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.MarkLastUsed(context.Background(), testNs(), time.Now().UTC())

	require.Error(t, err)
}

func TestSetChannel_SendsCommand(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	expected := testArrow()

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(context.Context, domain.Namespace, string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, expected, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))

	err := cat.SetChannel(context.Background(), ns, "rc")
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "rc", got.Channel)
}

// Nothing tracks a channel for an arrow that is not in the catalog, so a
// change asked for on an unknown namespace is a state violation rather than
// a silent no-op.
func TestSetChannel_UnknownNamespace_Errors(t *testing.T) {
	axArrow := newTestAsynxArrow(t)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.SetChannel(context.Background(), testNs(), "rc")

	require.Error(t, err)
}

func TestForget_UsesAsynxArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.Forget(context.Background(), ns)
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUpdateManifest_SendsCommand(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	updated := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Updated Arrow"},
	}
	err = cat.UpdateManifest(context.Background(), ns, updated)
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Updated Arrow", got.Name)
}

func TestResolveConstraint_DelegatesToManifold(t *testing.T) {
	m := &mocks.Manifold{ResolveConstraintResult: "v1.0.0"}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	ref, err := cat.ResolveConstraint(context.Background(), testNs(), "^v1")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", ref)
}

func TestResolveLatestStable_DelegatesToManifold(t *testing.T) {
	m := &mocks.Manifold{ResolveLatestStableRef: "stable-1.1"}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	ref, err := cat.ResolveLatestStable(context.Background(), testNs())
	require.NoError(t, err)
	assert.Equal(t, "stable-1.1", ref)
}

func TestListChannels_DelegatesToManifold(t *testing.T) {
	m := &mocks.Manifold{
		ListChannelsResult: []manifold.ChannelInfo{
			{Name: "stable", Kind: "ordered", Latest: "v1.0.0", Count: 1, Members: []string{"v1.0.0"}},
			{Name: "main", Kind: "pointer", Latest: "main"},
		},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)

	got, err := cat.ListChannels(context.Background(), testNs())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "stable", got[0].Name)
	assert.Equal(t, "ordered", got[0].Kind)
	assert.Equal(t, "v1.0.0", got[0].Latest)
	assert.Equal(t, 1, got[0].Count)
	assert.Equal(t, []string{"v1.0.0"}, got[0].Members)
	assert.Equal(t, "main", got[1].Name)
	assert.Equal(t, "pointer", got[1].Kind)
}

func TestListChannels_ManifoldError_Propagates(t *testing.T) {
	wantErr := errors.New("list tags: connection refused")
	m := &mocks.Manifold{ListChannelsErr: wantErr}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)

	_, err := cat.ListChannels(context.Background(), testNs())
	require.ErrorIs(t, err, wantErr)
}

func TestValidateManifest_Valid(t *testing.T) {
	arrow := testArrow()
	arrow.Targets = map[domain.OS]domain.Target{domain.OSDarwinARM64: {}}
	m := &mocks.Manifold{ParseArrowResult: arrow}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	result, err := cat.ValidateManifest(context.Background(), []byte("data"))
	require.NoError(t, err)
	assert.True(t, result.Valid)
}

func TestValidateManifest_Invalid(t *testing.T) {
	m := &mocks.Manifold{ParseArrowErr: errors.New("bad manifest")}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	result, err := cat.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Message, "bad manifest")
}

func TestAdd_NewArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	expected := testArrow()
	expected.UserInstalled = true

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, expected, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns, models.AddOptions{})
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, exists)
}

// Regression: Add builds the AddArrow command from explicit fields, not by
// passing the resolved *domain.Arrow through, so a field the command struct
// doesn't list is silently dropped no matter what ResolveForInstall computed
// — this caught RefIsBranch/RefCommitSHA never reaching the persisted
// aggregate despite store.go stamping them correctly.
func TestAdd_CarriesRefIsBranchAndRefCommitSHAThrough(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	resolved := testArrow()
	resolved.RefIsBranch = true
	resolved.RefCommitSHA = "abc123"

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(context.Context, domain.Namespace, string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, resolved, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.RefIsBranch)
	assert.Equal(t, "abc123", got.RefCommitSHA)
}

// Regression: Add builds the AddArrow command from explicit fields, the same
// way TestAdd_CarriesRefIsBranchAndRefCommitSHAThrough guards for
// RefIsBranch/RefCommitSHA — Channel needs the same explicit forwarding or a
// resolved channel would silently never reach the persisted aggregate.
func TestAdd_CarriesChannelThrough(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	resolved := testArrow()
	resolved.Channel = "rc"
	var gotChannel string

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, channel string) (domain.Namespace, *domain.Arrow, string, error) {
			gotChannel = channel
			return ns, resolved, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{Channel: "rc"}))

	assert.Equal(t, "rc", gotChannel, "Add must forward opts.Channel into ResolveForInstall")

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "rc", got.Channel)
}

func TestAdd_ExistingUserInstalled_Noop(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Add arrow first with UserInstalled=true
	_, err := axArrow.Send(context.Background(), addArrowCmdUserInstalled(ns))
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, &domain.Arrow{Namespace: ns, UserInstalled: true}, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err = cat.Add(context.Background(), ns, models.AddOptions{})
	require.NoError(t, err) // Should be no error - existing user-installed arrow is a no-op
}

func TestAdd_ExistingNotUserInstalled_SetsUserInstalled(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Add arrow as a dep (UserInstalled=false)
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	arrow := testArrow()
	arrow.UserInstalled = false

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, arrow, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err = cat.Add(context.Background(), ns, models.AddOptions{})
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.UserInstalled)
}

func TestRemove_NotFound(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.Remove(context.Background(), testNs())
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AbsentRuntime_Success(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.Remove(context.Background(), ns)
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestShutdown_DelegatesToAsynxArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestUpgradeVersion_FetchesAndAdds(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "Updated"}}
	v := &mocks.Vault{
		GetArrowErr:    errors.New("not cached"),
		DeleteArrowErr: nil,
	}

	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	got, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false, false)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
}

// TestUpgradeVersion_NoVaultEntryForOldNs_SucceedsCleanly proves the real,
// filesystem-backed hardening this feature added: UpgradeVersion (through
// vault.RenameArrow) must not fail just because oldNs was never cached in
// the vault at all — TTL-swept, never cached, or any other benign reason —
// since PutArrow writes newNs's own entry fresh right afterward regardless.
// This uses a real vault.Vault rather than the mock: the mock's RenameArrow
// always succeeds by default, so it cannot reproduce the "no cached meta
// file" condition the real vault hit in production.
func TestUpgradeVersion_NoVaultEntryForOldNs_SucceedsCleanly(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "Updated"}}

	v, err := vault.New(t.TempDir(), t.TempDir(), time.Hour)
	require.NoError(t, err)

	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	got, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false, false)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
}

// TestUpgradeVersion_CachesTheNewRefWithIndexMetadata guards the same defect
// Seed had: caching a manifest without Meta writes the bytes to disk but no
// vault index row, so the upgraded ref exists on disk and is invisible to the
// vault lane of search. A call count cannot distinguish that from a good write,
// which is why the assertion is on the metadata.
func TestUpgradeVersion_CachesTheNewRefWithIndexMetadata(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{
		Namespace: newNs,
		ArrowMeta: domain.ArrowMeta{Name: "Updated"},
		Targets:   map[domain.OS]domain.Target{domain.OSDarwinARM64: {}},
	}
	v := &mocks.Vault{GetArrowErr: errors.New("not cached")}
	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	_, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false, false)
	require.NoError(t, err)

	require.NotEmpty(t, v.PutArrowFiles, "the upgraded ref must be cached")
	cached := v.PutArrowFiles[len(v.PutArrowFiles)-1]
	require.NotNil(t, cached.Meta,
		"a manifest cached without Meta is unreachable through the vault lane of search")
	assert.Equal(t, "Updated", cached.Meta.Arrow.Name)
	assert.Equal(t, []domain.OS{domain.OSDarwinARM64}, cached.Meta.OS)
}

// TestUpgradeVersionSeeded_NeverCallsManifold pins the whole point of this
// method: quiver.core's own self-registration must never depend on network
// reachability just to record which version of itself is running. A
// Manifold whose ResolveArrow always errors would fail this test the moment
// UpgradeVersionSeeded touched it.
func TestUpgradeVersionSeeded_NeverCallsManifold(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	oldNs := testNs()
	newNs := oldNs.BareNamespace().WithRef("v1.1.0")
	arrow := testArrow()
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ParseArrowResult: arrow,
		ResolveArrowErr:  errors.New("must never be reached"),
	}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.UpgradeVersionSeeded(context.Background(), oldNs, newNs, []byte("embedded manifest bytes"))
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), newNs.String())
	require.NoError(t, err)
	assert.Equal(t, newNs, got.Namespace)
	assert.Equal(t, oldNs, got.UpgradedFromNs)
	assert.True(t, got.AlreadyReady)
	assert.Empty(t, got.InstalledConstraint)
}

func TestUpgradeVersionSeeded_InvalidManifest_ReturnsError(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	m := &mocks.Manifold{ParseArrowErr: errors.New("bad manifest")}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, m)
	err := cat.UpgradeVersionSeeded(context.Background(), testNs(), testNs().BareNamespace().WithRef("v2"), []byte("bad"))
	require.Error(t, err)
}

func TestUpgradeVersionSeeded_VaultPutError_ReturnsError(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	arrow := testArrow()
	v := &mocks.Vault{PutArrowErr: errors.New("put failed")}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.UpgradeVersionSeeded(context.Background(), testNs(), testNs().BareNamespace().WithRef("v2"), []byte("data"))
	require.Error(t, err)
}

// ─── Internal command helpers ──────────────────────────────────────────────

// These replicate the catalog commands to seed test state directly.

type addArrowCommand struct {
	ns            domain.Namespace
	userInstalled bool
}

func (c addArrowCommand) AggregateID() string  { return c.ns.String() }
func (c addArrowCommand) EventName() string    { return "arrow.added." + c.ns.String() }
func (c addArrowCommand) ShouldSnapshot() bool { return true }
func (c addArrowCommand) Validate(current *domain.Arrow) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	return nil
}

func (c addArrowCommand) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{Namespace: c.ns, UserInstalled: c.userInstalled}
}

func addArrowCmd(ns domain.Namespace) asynxModels.Command[domain.Arrow] {
	return addArrowCommand{ns: ns, userInstalled: false}
}

func addArrowCmdUserInstalled(ns domain.Namespace) asynxModels.Command[domain.Arrow] {
	return addArrowCommand{ns: ns, userInstalled: true}
}

func TestAddDep_NewArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := testArrow()
	arrow.UserInstalled = false

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.AddDep(context.Background(), ns, arrow, "")
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, exists)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, got.UserInstalled)
}

func TestAddDep_ExistingArrow_AlreadyUserInstalled_Noop(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Seed user-installed
	_, err := axArrow.Send(context.Background(), addArrowCmdUserInstalled(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.AddDep(context.Background(), ns, testArrow(), "")
	require.NoError(t, err) // Should be no-op
}

func TestSeed_ValidManifest(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	arrow := testArrow()
	arrow.UserInstalled = true
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), ns, []byte("valid manifest content"))
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSeed_InvalidNamespace(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	v := &mocks.Vault{}
	m := &mocks.Manifold{}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	// Empty namespace is invalid
	err := cat.Seed(context.Background(), domain.Namespace(""), []byte("data"))
	require.Error(t, err)
}

func TestSeed_InvalidManifest(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowErr: errors.New("bad manifest")}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), testNs(), []byte("bad"))
	require.Error(t, err)
}

func TestSeed_ExistingArrow_SetsUserInstalled(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Add the arrow first (not user-installed)
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	arrow := testArrow()
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err = cat.Seed(context.Background(), ns, []byte("valid manifest"))
	require.NoError(t, err)

	// Arrow should now be marked as user-installed
	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, got.UserInstalled)
}

func TestValidateManifest_InvalidWithRuleErrors(t *testing.T) {
	m := &mocks.Manifold{ParseArrowErr: errors.New("some generic parse error")}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	result, err := cat.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
}

func TestUpgradeVersion_RuntimeAlreadyExists_SkipsVault(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "Updated"}}
	v := &mocks.Vault{} // vault should not be called for rename/put
	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	// runtimeAlreadyExists=true → skips vault rename
	got, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true, false)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "v1.1.0", got.Namespace.Ref(), "the upgraded arrow takes its version from the new ref")
	// Vault rename should NOT have been called when runtime exists
	assert.Equal(t, 0, v.PutArrowCalls)
}

func TestValidateManifest_RuleErrors(t *testing.T) {
	ruleErr := ruleset.RuleErrors{
		{Field: "targets", Rule: "missing_lifecycle", Message: "no install step"},
	}
	m := &mocks.Manifold{ParseArrowErr: ruleErr}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	result, err := cat.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Equal(t, "missing_lifecycle", result.Errors[0].Rule)
	assert.Equal(t, "targets", result.Errors[0].Field)
}

func TestAdd_ResolveForInstallError(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, ns domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", errors.New("resolve error")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	err := cat.Add(context.Background(), testNs(), models.AddOptions{})
	require.Error(t, err)
}

// Seeded bytes have no remote to ask for a ref, and nothing inside a manifest
// is one: the caller has to say which ref these bytes are.
func TestSeed_BareNamespace_IsRejected(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	arrow := testArrow()
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	bareNs := testNs().BareNamespace() // no ref
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), bareNs, []byte("data"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
	assert.Contains(t, err.Error(), string(bareNs))
	assert.Equal(t, 0, v.PutArrowCalls, "nothing is written for a namespace with no ref")

	exists, err := axArrow.Exists(context.Background(), bareNs.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

// The ref the caller seeds under is the arrow's version, so it has to win over
// whatever ref the parsed bytes happen to name themselves.
func TestSeed_VersionComesFromTheRef(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	arrow := testArrow()
	arrow.Namespace = testNs().BareNamespace().WithRef("nightly")
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	ns := testNs().BareNamespace().WithRef("v3.1.0")
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	require.NoError(t, cat.Seed(context.Background(), ns, []byte("data")))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "v3.1.0", got.Namespace.Ref())
}

func TestSeed_VaultPutError(t *testing.T) {
	v := &mocks.Vault{PutArrowErr: errors.New("vault error")}
	m := &mocks.Manifold{ParseArrowResult: testArrow()}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), v, m)
	err := cat.Seed(context.Background(), testNs(), []byte("data"))
	require.Error(t, err)
}

func TestSeed_InvalidNamespace_Error(t *testing.T) {
	m := &mocks.Manifold{ParseArrowResult: testArrow()}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	// Use a namespace with no version or bare that validates to error
	err := cat.Seed(context.Background(), domain.Namespace(""), []byte("data"))
	require.Error(t, err)
}

func TestUpgradeVersion_ManifoldError(t *testing.T) {
	m := &mocks.Manifold{ResolveArrowErr: errors.New("fetch failed")}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), nil, m)
	_, err := cat.UpgradeVersion(context.Background(), testNs(), testNs().BareNamespace().WithRef("v2"), "^v1", false, false)
	require.Error(t, err)
}

func TestUpgradeVersion_VaultPutError(t *testing.T) {
	newNs := testNs().BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{Namespace: newNs}
	v := &mocks.Vault{PutArrowErr: errors.New("put failed")}
	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), v, m)
	_, err := cat.UpgradeVersion(context.Background(), testNs(), newNs, "^v1", false, false)
	require.Error(t, err)
}

func TestUpgradeVersion_VaultRenameError(t *testing.T) {
	newNs := testNs().BareNamespace().WithRef("v1.1.0")
	newArrow := &domain.Arrow{Namespace: newNs}
	v := &mocks.Vault{RenameArrowErr: errors.New("rename failed")}
	m := &mocks.Manifold{
		ResolveArrowResult:   newArrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), v, m)
	_, err := cat.UpgradeVersion(context.Background(), testNs(), newNs, "^v1", false, false)
	require.Error(t, err)
}

func TestRemove_NotFound_NoSeededArrow(t *testing.T) {
	// axArrow with no arrow seeded → Remove returns ErrNotFound
	axArrow := newTestAsynxArrow(t)
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.Remove(context.Background(), testNs())
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAddArrowCommand_GetError(t *testing.T) {
	// To test the getErr != ErrNotFound path, we'd need an asynx that
	// returns something other than ErrNotFound. This is hard to mock with real asynx.
	// Instead test the normal path through AddDep with a bad asynx.
	// This is a best-effort test for the existing branch.
	axArrow := newTestAsynxArrow(t)
	ns := testNs()
	// Seed the arrow as not user-installed
	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	arrow := testArrow()
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	// AddDep with non-user-installed → SetUserInstalled path
	err = cat.AddDep(context.Background(), ns, arrow, "")
	require.NoError(t, err)

	// After SetUserInstalled, arrow should be user-installed
	got, getErr := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, getErr)
	assert.True(t, got.UserInstalled)
}

// ─── arrowRepo.New coverage ─────────────────────────────────────────────────────

func TestNew_Success(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	cat, err := arrowRepo.New(db, axArrow, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, cat)
}

func TestNew_CQRSError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	_, err = arrowRepo.New(db, axArrow, nil, nil, nil)
	require.Error(t, err)
}

func TestNew_ShutdownBeforeInit_ReturnsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)

	_ = axArrow.Shutdown(context.Background())
	_, err = arrowRepo.New(db, axArrow, nil, nil, nil)
	require.Error(t, err)
}

func TestNew_WithVault_OnForgetRegistered(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	v := &mocks.Vault{}
	cat, err := arrowRepo.New(db, axArrow, v, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, cat)
}

// ─── New: forget projection error ─────────────────────────────────────────────

func TestNew_ForgetProjectionError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := &arrowMocks.AsynxArrow{
		OnForgetFn: func(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
			return "", errors.New("on-forget error")
		},
	}

	v := &mocks.Vault{}
	_, err = arrowRepo.New(db, axArrow, v, nil, nil)
	require.Error(t, err)
}

func TestNew_TopicSubscribeError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := &arrowMocks.AsynxArrow{
		SubscribeFn: func(
			_ string,
			_ asynxModels.ProjectionHandler[domain.Arrow],
			_ ...asynxModels.SubscriptionOpt[domain.Arrow],
		) (string, error) {
			return "", errors.New("subscribe error")
		},
	}

	_, err = arrowRepo.New(db, axArrow, nil, nil, nil)
	require.Error(t, err)
}

// ─── Seed: addArrowCommand returns non-ErrAlreadyExists error ─────────────────

func TestSeed_AddArrowError_NonErrAlreadyExists(t *testing.T) {
	ns := testNs()
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, errors.New("send error")
		},
	}
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Test"}},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), ns, []byte("raw"))
	require.Error(t, err)
}

// ─── UpgradeVersion: addArrowCommand returns non-ErrAlreadyExists error ───────

func TestUpgradeVersion_AddArrowError(t *testing.T) {
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v2.0.0")
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, errors.New("add error")
		},
	}
	arrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "New"}}
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	_, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true, false) // skip vault ops
	require.Error(t, err)
}

// ─── addArrowCommand: non-ErrNotFound getErr ──────────────────────────────────

func TestAddArrow_GetReturnsNonErrNotFoundError(t *testing.T) {
	ns := testNs()
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, errors.New("connection error") // not ErrNotFound
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns, models.AddOptions{})
	require.Error(t, err)
}

// ─── addArrowCommand: ErrValidation/ErrPipelineFailed → ErrAlreadyExists ──────

func TestAddArrow_SendValidationError_ReturnsAlreadyExists(t *testing.T) {
	ns := testNs()
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns, models.AddOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrAlreadyExists))
}

func TestAddArrow_SendPipelineFailedError_ReturnsAlreadyExists(t *testing.T) {
	ns := testNs()
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrPipelineFailed
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns, models.AddOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrAlreadyExists))
}

func TestAddArrow_SendGenericError(t *testing.T) {
	ns := testNs()
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, errors.New("generic send error")
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns, models.AddOptions{})
	require.Error(t, err)
}

// ─── Remove: Exists error ─────────────────────────────────────────────────────

func TestRemove_ExistsError(t *testing.T) {
	axArrow := &arrowMocks.AsynxArrow{
		ExistsFn: func(ctx context.Context, id string) (bool, error) {
			return false, errors.New("db error")
		},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err := cat.Remove(context.Background(), testNs())
	require.Error(t, err)
}

// ─── Seed: ErrAlreadyExists → UpdateArrowManifest ────────────────────────────

func TestSeed_AlreadyExists_UpdatesManifest(t *testing.T) {
	ns := testNs()
	callCount := 0
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			if callCount == 0 {
				callCount++
				return domain.Arrow{Namespace: ns}, nil // exists, UserInstalled=false
			}
			return domain.Arrow{}, nil
		},
		SendFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Seeded"}},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), ns, []byte("raw"))
	// addArrowCommand finds existing non-user-installed → sends SetUserInstalled (success)
	// Then returns nil (not ErrAlreadyExists) → Seed returns nil
	require.NoError(t, err)
}

func TestSeed_AlreadyExists_ErrAlreadyExists_UpdatesManifest(t *testing.T) {
	ns := testNs()
	var sentManifest arrowcmds.UpdateArrowManifest
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendWaitFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			// First send (AddArrow) returns ErrValidation → ErrAlreadyExists in addArrowCommand
			// Second send (UpdateArrowManifest) returns nil
			if len(cmd.EventName()) > 12 && cmd.EventName()[:12] == "arrow.added." {
				return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
			}
			sentManifest = cmd.(arrowcmds.UpdateArrowManifest)
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Seeded"}, Readme: "# Docs"},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), ns, []byte("raw"))
	require.NoError(t, err)
	assert.Equal(t, "# Docs", sentManifest.Readme, "the update falls back to re-sending the seeded manifest's readme")
}

// ─── UpgradeVersion: DeleteArrow error is logged (soft-fail) ──────────────────

func TestUpgradeVersion_DeleteArrowError_Continues(t *testing.T) {
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v2.0.0")
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	v := &mocks.Vault{
		DeleteArrowErr: errors.New("delete error"), // soft-fail
		RenameArrowErr: nil,
		PutArrowErr:    nil,
	}
	arrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "New"}}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	_, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false, false)
	require.NoError(t, err) // DeleteArrow error is logged, not returned
}

// ─── UpgradeVersion: runtimeAlreadyExists=true path ──────────────────────────

func TestUpgradeVersion_RuntimeAlreadyExists_SkipsVaultOps(t *testing.T) {
	ns := testNs()
	newNs := ns.BareNamespace().WithRef("v2.0.0")
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	v := &mocks.Vault{
		RenameArrowErr: errors.New("rename should not be called"), // should not be reached
	}
	arrow := &domain.Arrow{Namespace: newNs, ArrowMeta: domain.ArrowMeta{Name: "New"}}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	result, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true, false) // runtimeAlreadyExists=true
	require.NoError(t, err)
	require.NotNil(t, result)
}

// ─── Callbacks: OnArrowAdded ──────────────────────────────────────────────────

func TestOnArrowAdded_CallbackFiresOnAdd(t *testing.T) {
	ns := testNs()
	arrow := testArrow()
	callbackFired := false
	var capturedNs domain.Namespace
	var capturedArrow domain.Arrow

	axArrow := newTestAsynxArrow(t)
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{
		GetFn: func(ctx context.Context, id domain.Namespace) (*domain.Arrow, error) {
			return arrow, nil
		},
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, arrow, "", nil
		},
	}, axArrow)

	// Register callback
	err := cat.OnArrowAdded(func(ctx context.Context, captureNs domain.Namespace, captureArrow domain.Arrow) error {
		callbackFired = true
		capturedNs = captureNs
		capturedArrow = captureArrow
		return nil
	})
	require.NoError(t, err)

	// Trigger by adding an arrow
	err = cat.Add(context.Background(), ns, models.AddOptions{})
	require.NoError(t, err)
	axArrow.WaitPublish()

	require.True(t, callbackFired, "callback should have fired")
	assert.Equal(t, ns, capturedNs)
	assert.Equal(t, ns, capturedArrow.Namespace)
}

// ─── Callbacks: OnArrowRemoved ────────────────────────────────────────────────

func TestOnArrowRemoved_CallbackFiresOnRemove(t *testing.T) {
	ns := testNs()
	callbackFired := false
	var capturedNs domain.Namespace

	axArrow := newTestAsynxArrow(t)
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}, axArrow)

	// Register callback
	err := cat.OnArrowRemoved(func(ctx context.Context, captureNs domain.Namespace) error {
		callbackFired = true
		capturedNs = captureNs
		return nil
	})
	require.NoError(t, err)

	// First add an arrow so we can remove it
	err = cat.Add(context.Background(), ns, models.AddOptions{})
	require.NoError(t, err)
	axArrow.WaitPublish()

	// Now remove it
	err = cat.Remove(context.Background(), ns)
	require.NoError(t, err)
	axArrow.WaitPublish()

	require.True(t, callbackFired, "callback should have fired")
	assert.Equal(t, ns, capturedNs)
}

// ─── Callbacks: error path (callback returns error) ──────────────────────────

// emitArrowCmd emits any named event without side effects on the Arrow aggregate.
type emitArrowCmd struct {
	ns        domain.Namespace
	eventName string
}

func (c emitArrowCmd) AggregateID() string            { return c.ns.String() }
func (c emitArrowCmd) EventName() string              { return c.eventName }
func (c emitArrowCmd) ShouldSnapshot() bool           { return false }
func (c emitArrowCmd) Validate(_ *domain.Arrow) error { return nil }
func (c emitArrowCmd) EmitEvent(current *domain.Arrow) domain.Arrow {
	if current != nil {
		return *current
	}
	return domain.Arrow{Namespace: c.ns}
}

var _ asynxModels.Command[domain.Arrow] = emitArrowCmd{}

func TestOnArrowAdded_ErrorCallbackLogged(t *testing.T) {
	ns := testNs()
	cbErr := errors.New("callback error")
	errored := make(chan struct{}, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{}, axArrow)

	require.NoError(t, cat.OnArrowAdded(func(_ context.Context, _ domain.Namespace, _ domain.Arrow) error {
		errored <- struct{}{}
		return cbErr
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.added." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case <-errored:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not called")
	}
}

// ─── Callbacks: OnArrowUpdated ────────────────────────────────────────────────

func TestOnArrowUpdated_CallbackFires(t *testing.T) {
	ns := testNs()
	called := make(chan domain.Namespace, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{}, axArrow)

	require.NoError(t, cat.OnArrowUpdated(func(_ context.Context, n domain.Namespace, _ *domain.Arrow) error {
		called <- n
		return nil
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.updated." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case got := <-called:
		assert.Equal(t, ns, got)
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowUpdated callback not called")
	}
}

func TestOnArrowUpdated_ErrorCallbackLogged(t *testing.T) {
	ns := testNs()
	errored := make(chan struct{}, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{}, axArrow)

	require.NoError(t, cat.OnArrowUpdated(func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
		errored <- struct{}{}
		return errors.New("update cb error")
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.updated." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case <-errored:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not called")
	}
}

// ─── Callbacks: OnArrowRemoved error path ────────────────────────────────────

func TestOnArrowRemoved_ErrorCallbackLogged(t *testing.T) {
	ns := testNs()
	errored := make(chan struct{}, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}, axArrow)

	require.NoError(t, cat.OnArrowRemoved(func(_ context.Context, _ domain.Namespace) error {
		errored <- struct{}{}
		return errors.New("removed cb error")
	}))

	// Add then remove to trigger OnForget.
	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))
	axArrow.WaitPublish()
	require.NoError(t, cat.Remove(context.Background(), ns))
	axArrow.WaitPublish()

	select {
	case <-errored:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not called")
	}
}

// ─── Callbacks: OnArrowUpgraded ──────────────────────────────────────────────

func TestOnArrowUpgraded_CallbackFires(t *testing.T) {
	ns := testNs()
	called := make(chan domain.Arrow, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{}, axArrow)

	require.NoError(t, cat.OnArrowUpgraded(func(_ context.Context, a domain.Arrow) error {
		called <- a
		return nil
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.upgraded." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case got := <-called:
		assert.Equal(t, ns, got.Namespace)
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowUpgraded callback not called")
	}
}

func TestOnArrowUpgraded_ErrorCallbackLogged(t *testing.T) {
	ns := testNs()
	errored := make(chan struct{}, 1)

	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat := newProjectingTestable(t, &arrowStoreMocks.MockCQRS{}, axArrow)

	require.NoError(t, cat.OnArrowUpgraded(func(_ context.Context, _ domain.Arrow) error {
		errored <- struct{}{}
		return errors.New("upgraded cb error")
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.upgraded." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case <-errored:
	case <-time.After(2 * time.Second):
		t.Fatal("callback not called")
	}
}

func TestSearch_DelegatesToCQRS(t *testing.T) {
	hit := models.CatalogHit{
		Namespace:  testNs().BareNamespace(),
		Metadata:   *testArrow(),
		Refs:       []string{"v1.0.0"},
		Provenance: models.ProvenanceInstalled,
	}
	var seen models.SearchQuery
	r := &arrowStoreMocks.MockCQRS{
		SearchFn: func(ctx context.Context, q models.SearchQuery) ([]models.CatalogHit, error) {
			seen = q
			return []models.CatalogHit{hit}, nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	q := models.SearchQuery{Text: "test", OS: domain.OSLinuxAMD64, Limit: 7}
	got, err := cat.Search(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, hit, got[0])
	assert.Equal(t, q, seen)
}

func TestSearch_Error(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		SearchFn: func(ctx context.Context, q models.SearchQuery) ([]models.CatalogHit, error) {
			return nil, errors.New("db error")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	_, err := cat.Search(context.Background(), models.SearchQuery{Text: "test"})
	require.Error(t, err)
}

// ─── Projection ordering ─────────────────────────────────────────────────────

// recordingHub counts catalog announcements so a test can tell whether an arrow
// was announced before it was readable.
type recordingHub struct {
	mu     sync.Mutex
	events []apphub.ArrowEvent
}

func (h *recordingHub) BroadcastArrow(e apphub.ArrowEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, e)
}

func (h *recordingHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime) {}

func (h *recordingHub) BroadcastCollection(_ apphub.CollectionEvent) {}

func (h *recordingHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

func (h *recordingHub) kinds() []apphub.CatalogEventKind {
	h.mu.Lock()
	defer h.mu.Unlock()
	kinds := make([]apphub.CatalogEventKind, 0, len(h.events))
	for _, e := range h.events {
		kinds = append(kinds, e.Kind)
	}
	return kinds
}

// observation is what a reaction could see of the rest of the projection at the
// moment it ran.
type observation struct {
	projected  int32
	broadcasts int
}

func newProjectingTestableWithHub(
	t *testing.T,
	r *arrowStoreMocks.MockCQRS,
	axArrow asynx.Asynx[domain.Arrow],
	hub apphub.WebSocketHub,
) arrowRepo.Arrow {
	t.Helper()
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, nil, nil, hub)
	require.NoError(t, err)
	return cat
}

// A reaction is what makes an arrow usable — the dependency graph above all —
// so it has to have finished before anything can read the arrow or be told it
// exists.
func TestProjectAdded_ReactionsRunBeforeReadModelAndBroadcast(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)

	seen := make(chan observation, 1)
	require.NoError(t, cat.OnArrowAdded(func(_ context.Context, _ domain.Namespace, _ domain.Arrow) error {
		seen <- observation{projected: projected.Load(), broadcasts: hub.count()}
		return nil
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.added." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case got := <-seen:
		assert.Zero(t, got.projected, "the arrow was readable before its reactions had run")
		assert.Zero(t, got.broadcasts, "the arrow was announced before its reactions had run")
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowAdded reaction never ran")
	}

	assert.Equal(t, int32(1), projected.Load())
	assert.Equal(t, 1, hub.count())
}

func TestProjectUpdated_ReactionsRunBeforeReadModelAndBroadcast(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)

	seen := make(chan observation, 1)
	require.NoError(t, cat.OnArrowUpdated(func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
		seen <- observation{projected: projected.Load(), broadcasts: hub.count()}
		return nil
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.updated." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case got := <-seen:
		assert.Zero(t, got.projected)
		assert.Zero(t, got.broadcasts)
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowUpdated reaction never ran")
	}

	assert.Equal(t, int32(1), projected.Load())
}

func TestProjectUpgraded_ReactionsRunBeforeReadModelAndBroadcast(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)

	seen := make(chan observation, 1)
	require.NoError(t, cat.OnArrowUpgraded(func(_ context.Context, _ domain.Arrow) error {
		seen <- observation{projected: projected.Load(), broadcasts: hub.count()}
		return nil
	}))

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.upgraded." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	select {
	case got := <-seen:
		assert.Zero(t, got.projected)
		assert.Zero(t, got.broadcasts)
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowUpgraded reaction never ran")
	}

	assert.Equal(t, int32(1), projected.Load())
}

func TestProjectInstalled_WritesReadModelAndAnnounces(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)
	require.NotNil(t, cat)

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.installed." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.Equal(t, int32(1), projected.Load())
	assert.Equal(t, []apphub.CatalogEventKind{apphub.CatalogUpserted}, hub.kinds())
}

// The cleared stamp has to reach the read model too — that is where the API
// answers installed_at from.
func TestProjectUninstalled_WritesReadModelAndAnnounces(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)
	require.NotNil(t, cat)

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.uninstalled." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.Equal(t, int32(1), projected.Load())
	assert.Equal(t, []apphub.CatalogEventKind{apphub.CatalogUpserted}, hub.kinds())
}

// A version check that finds a diff has to reach the read model too — that is
// where the API answers outdated/recommended_ref from. This also guards
// against the easiest way to make the whole feature silently inert: wiring
// the command's EmitEvent correctly (tested in commands_test.go) but
// forgetting to subscribe arrowService to its topic.
func TestProjectVersionChecked_WritesReadModelAndAnnounces(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var projected atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			projected.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)
	require.NotNil(t, cat)

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.version_checked." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.Equal(t, int32(1), projected.Load())
	assert.Equal(t, []apphub.CatalogEventKind{apphub.CatalogUpserted}, hub.kinds())
}

// An arrow that could not be written is not there to be read, so announcing it
// would be announcing nothing.
func TestProjectAdded_ReadModelFailureIsNotAnnounced(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, _ domain.Arrow) error {
			return errors.New("disk full")
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)
	require.NotNil(t, cat)

	_, err := axArrow.Send(context.Background(), emitArrowCmd{ns: ns, eventName: "arrow.added." + ns.String()})
	require.NoError(t, err)
	axArrow.WaitPublish()

	assert.Zero(t, hub.count(), "a read model that was never written must not be announced")
}

// Removal is the mirror: the row goes first, because an arrow stays readable
// only while the edges its removal guard consults are still there.
func TestProjectForgotten_ReadModelClearedBeforeReactions(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	var forgotten atomic.Int32
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
		ProjectForgetFn: func(_ context.Context, _ domain.Arrow) error {
			forgotten.Add(1)
			return nil
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)

	seen := make(chan observation, 1)
	require.NoError(t, cat.OnArrowRemoved(func(_ context.Context, _ domain.Namespace) error {
		seen <- observation{projected: forgotten.Load(), broadcasts: hub.count()}
		return nil
	}))

	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))
	axArrow.WaitPublish()
	require.NoError(t, cat.Remove(context.Background(), ns))
	axArrow.WaitPublish()

	select {
	case got := <-seen:
		assert.Equal(t, int32(1), got.projected,
			"the arrow was still readable while its edges were being torn down")
		assert.Equal(t, 1, got.broadcasts,
			"only the add should have been announced by the time the reaction ran")
	case <-time.After(2 * time.Second):
		t.Fatal("OnArrowRemoved reaction never ran")
	}

	assert.Equal(t, []apphub.CatalogEventKind{apphub.CatalogUpserted, apphub.CatalogRemoved}, hub.kinds())
}

func TestProjectForgotten_ReadModelFailureKeepsReactionsAndBroadcast(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	hub := &recordingHub{}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
		ProjectForgetFn: func(_ context.Context, _ domain.Arrow) error {
			return errors.New("db closed")
		},
	}
	cat := newProjectingTestableWithHub(t, r, axArrow, hub)

	var reacted atomic.Int32
	require.NoError(t, cat.OnArrowRemoved(func(_ context.Context, _ domain.Namespace) error {
		reacted.Add(1)
		return nil
	}))

	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))
	axArrow.WaitPublish()
	require.NoError(t, cat.Remove(context.Background(), ns))
	axArrow.WaitPublish()

	assert.Zero(t, reacted.Load(),
		"edges must survive a row that is still readable")
	assert.Equal(t, []apphub.CatalogEventKind{apphub.CatalogUpserted}, hub.kinds())
}

// workDirVault records the work dirs the forget projection releases, and fails
// the release so the projection's tolerance of that failure is exercised too.
type workDirVault struct {
	*mocks.Vault
	mu      sync.Mutex
	deleted []domain.Namespace
}

func (v *workDirVault) DeleteWorkDir(
	_ context.Context,
	ns domain.Namespace,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.deleted = append(v.deleted, ns)
	return errors.New("workdir busy")
}

func (v *workDirVault) released() []domain.Namespace {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.Clone(v.deleted)
}

func TestProjectForgotten_ReleasesVaultWorkDir(t *testing.T) {
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	v := &workDirVault{Vault: &mocks.Vault{}}
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, v, nil, nil)
	require.NoError(t, err)

	require.NoError(t, cat.Add(context.Background(), ns, models.AddOptions{}))
	axArrow.WaitPublish()
	require.NoError(t, cat.Remove(context.Background(), ns))
	axArrow.WaitPublish()

	assert.Equal(t, []domain.Namespace{ns}, v.released(),
		"forgetting an arrow must release its work dir")
}

// ─── Add-time preinstalled detection ─────────────────────────────────────────

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

// preinstalledArrow is a manifest that opts into Add-time detection for the
// running platform, which is the only target Add ever looks at.
func preinstalledArrow(ns domain.Namespace) *domain.Arrow {
	return &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Preinstalled Arrow"},
		Targets: map[domain.OS]domain.Target{
			domain.CurrentOS(): {
				Lifecycle: domain.TargetLifecycle{
					Preinstalled: domainStep.StepList{
						domainStep.NewRunStep("Detect", "true", false, "10s", true),
					},
				},
			},
		},
	}
}

func resolvesTo(ns domain.Namespace, arrow *domain.Arrow) *arrowStoreMocks.MockCQRS {
	return &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace, _ string) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, arrow, "", nil
		},
	}
}

// detects is a probe that reports the arrow as already present.
func detects() arrowRepo.PreinstalledProbeFn {
	return func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
		return nil
	}
}

// TestArrowService_Add_PreinstalledDetected_MarksReadyDirectly is the contract
// this whole mechanism exists for: an arrow whose preinstalled block detects an
// existing install lands in the catalog as UserInstalled with its runtime
// already Ready — never installed, never Absent.
func TestArrowService_Add_PreinstalledDetected_MarksReadyDirectly(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(), detects(), runtimeRepo.MarkPreinstalled(axRuntime),
			runtimeRepo.ForgetPreinstalled(axRuntime),
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	arrow, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, arrow.UserInstalled)

	rt, err := axRuntime.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
	assert.Equal(t, ns, rt.Ref)
	assert.Nil(t, rt.Execution, "a detected arrow was never executed")
}

// TestArrowService_Add_PreinstalledDetected_NoAbsentWindow proves the ordering
// rather than arguing it. The OnArrowAdded callback runs inside the
// arrow.added projection, before the read model is written and before Add
// returns — the earliest instant anything in this process can observe the
// arrow at all. The runtime must already read Ready there, so there is no
// interleaving in which a caller sees UserInstalled with an Absent runtime.
func TestArrowService_Add_PreinstalledDetected_NoAbsentWindow(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	cat, err := arrowRepo.NewTestableProjecting(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(), detects(), runtimeRepo.MarkPreinstalled(axRuntime),
			runtimeRepo.ForgetPreinstalled(axRuntime),
		),
	)
	require.NoError(t, err)

	var (
		mu       sync.Mutex
		observed []domain.ArrowState
	)
	require.NoError(t, cat.OnArrowAdded(func(cbCtx context.Context, cbNs domain.Namespace, _ domain.Arrow) error {
		rt, getErr := axRuntime.Get(cbCtx, cbNs.String())
		mu.Lock()
		defer mu.Unlock()
		if errors.Is(getErr, asynxModels.ErrNotFound) {
			observed = append(observed, domain.ArrowStateAbsent)
			return nil
		}
		if getErr != nil {
			return getErr
		}
		observed = append(observed, rt.State)
		return nil
	}))

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	// No WaitPublish: addArrowCommand sends with SendWait, which asynx
	// documents as blocking until every subscribed projection has finished, so
	// the callback has necessarily already run by the time Add returns. Reading
	// without waiting is the point — it also pins down that a caller who reads
	// the instant Add returns cannot get ahead of the projection either.
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []domain.ArrowState{domain.ArrowStateReady}, observed,
		"the runtime must already be Ready the first instant the arrow is observable")
}

// TestArrowService_Add_NoPreinstalledBlock_UnchangedBehavior is the regression
// guard for every ordinary arrow in the system: with no preinstalled block
// declared, Add must not probe, must not touch the runtime aggregate, and must
// leave exactly what it left before — a UserInstalled catalog row with no
// ArrowRuntime at all (runtime.GetState maps that not-found to Absent).
func TestArrowService_Add_NoPreinstalledBlock_UnchangedBehavior(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	var probed, marked, forgotten atomic.Bool
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, testArrow()), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				probed.Store(true)
				return nil
			},
			func(cbCtx context.Context, cbNs domain.Namespace) error {
				marked.Store(true)
				return runtimeRepo.MarkPreinstalled(axRuntime)(cbCtx, cbNs)
			},
			func(_ context.Context, _ domain.Namespace) error {
				forgotten.Store(true)
				return nil
			},
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	assert.False(t, probed.Load(), "an arrow with no preinstalled block must never be probed")
	assert.False(t, marked.Load(), "an arrow with no preinstalled block must never touch the runtime")
	assert.False(t, forgotten.Load(), "an arrow with no preinstalled block must never touch the runtime")

	arrow, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, arrow.UserInstalled)

	exists, err := axRuntime.Exists(ctx, ns.String())
	require.NoError(t, err)
	assert.False(t, exists, "today's behaviour: a freshly added arrow has no runtime aggregate yet")
}

// TestArrowService_Add_PreinstalledNotDetected_UnchangedBehavior covers the
// other half of "unchanged": the block is declared, the probe runs, and it
// finds nothing. The arrow must land exactly as an ordinary one does.
func TestArrowService_Add_PreinstalledNotDetected_UnchangedBehavior(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	var marked, forgotten atomic.Bool
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				return errors.New("exit status 1")
			},
			func(_ context.Context, _ domain.Namespace) error {
				marked.Store(true)
				return nil
			},
			func(cbCtx context.Context, cbNs domain.Namespace) error {
				forgotten.Store(true)
				return runtimeRepo.ForgetPreinstalled(axRuntime)(cbCtx, cbNs)
			},
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	assert.False(t, marked.Load(), "a probe that finds nothing must not mark the runtime")
	assert.True(t, forgotten.Load(), "a negative probe must always clear any stale runtime before returning")

	arrow, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, arrow.UserInstalled)

	exists, err := axRuntime.Exists(ctx, ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestArrowService_Add_PreinstalledNotDetected_ClearsOrphanRuntime reproduces
// the exact sequence a prior review found: an earlier Add detected the arrow
// and marked its runtime Ready, then died before its own catalog row was ever
// written (a crash, a store error) — the software is then removed from the
// machine, and a later Add for the same namespace probes again and finds
// nothing. Without markIfPreinstalled clearing the orphan on that negative
// probe, this Add would silently inherit the earlier attempt's stale Ready:
// an arrow reported installed with no verified detection behind it, which is
// exactly what this mechanism exists to prevent.
func TestArrowService_Add_PreinstalledNotDetected_ClearsOrphanRuntime(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	// Simulate step 1: an earlier Add's probe detected the arrow and marked
	// the runtime Ready directly — the same call markIfPreinstalled itself
	// would have made — without ever going through Add, so no catalog row
	// exists for it. This is the orphan the earlier attempt's crash left
	// behind.
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(ctx, ns))
	orphan, err := axRuntime.Get(ctx, ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateReady, orphan.State, "the orphan must actually be Ready before this test proves anything")

	exists, err := axArrow.Exists(ctx, ns.String())
	require.NoError(t, err)
	require.False(t, exists, "the orphan's own Add never reached the catalog write")

	// Step 3: a later Add for the same namespace, whose own probe finds
	// nothing this time (the software was removed from the machine).
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				return errors.New("exit status 1")
			},
			runtimeRepo.MarkPreinstalled(axRuntime),
			runtimeRepo.ForgetPreinstalled(axRuntime),
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	// Step 4: the catalog row now exists (this Add's own doing), and the
	// orphan Ready runtime from the first, incomplete attempt must be gone —
	// not silently inherited.
	arrow, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, arrow.UserInstalled)

	stillExists, err := axRuntime.Exists(ctx, ns.String())
	require.NoError(t, err)
	assert.False(t, stillExists, "a negative probe must clear the orphan Ready runtime a prior incomplete Add left behind")
}

// TestArrowService_Add_PreinstalledForgetFails_AddsNothing: a negative probe
// that cannot clear a potential orphan runtime must fail the add rather than
// silently proceed on an answer it could not verify — the exact same
// all-or-nothing discipline TestArrowService_Add_PreinstalledMarkFails_AddsNothing
// already enforces for the positive-detection path.
func TestArrowService_Add_PreinstalledForgetFails_AddsNothing(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })

	forgetErr := errors.New("runtime store is down")
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				return errors.New("exit status 1")
			},
			func(_ context.Context, _ domain.Namespace) error { return nil },
			func(_ context.Context, _ domain.Namespace) error { return forgetErr },
		),
	)

	err := cat.Add(ctx, ns, models.AddOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, forgetErr)

	exists, err := axArrow.Exists(ctx, ns.String())
	require.NoError(t, err)
	assert.False(t, exists, "no catalog row may exist when a possible orphan runtime could not be cleared")
}

// TestArrowService_Add_PreinstalledMarkFails_AddsNothing keeps the two
// aggregates all-or-nothing. A runtime that cannot be marked Ready would leave
// the catalog row readable at Absent — the one state this mechanism exists to
// prevent — so the add fails instead and no row is written.
func TestArrowService_Add_PreinstalledMarkFails_AddsNothing(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })

	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(), detects(),
			func(_ context.Context, _ domain.Namespace) error {
				return errors.New("runtime store is down")
			},
			func(_ context.Context, _ domain.Namespace) error { return nil },
		),
	)

	require.Error(t, cat.Add(ctx, ns, models.AddOptions{}))

	exists, err := axArrow.Exists(ctx, ns.String())
	require.NoError(t, err)
	assert.False(t, exists, "no catalog row may exist when its runtime could not be marked Ready")
}

// TestArrowService_Add_PreinstalledAlreadyCatalogued_SkipsProbe keeps repeated
// Add calls cheap and non-destructive: quiver.desktop announces itself on every
// boot, and re-probing would spawn a subprocess each time and could stomp a
// runtime that has since moved on from Ready.
func TestArrowService_Add_PreinstalledAlreadyCatalogued_SkipsProbe(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })

	_, err := axArrow.Send(ctx, addArrowCmdUserInstalled(ns))
	require.NoError(t, err)

	var probed atomic.Bool
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				probed.Store(true)
				return nil
			},
			func(_ context.Context, _ domain.Namespace) error { return nil },
			func(_ context.Context, _ domain.Namespace) error { return nil },
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))
	assert.False(t, probed.Load(), "an arrow already in the catalog must not be re-probed")
}

// TestArrowService_Add_PreinstalledForeignPlatform_SkipsProbe: a preinstalled
// block declared only for another platform is not this machine's business.
func TestArrowService_Add_PreinstalledForeignPlatform_SkipsProbe(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })

	foreign := domain.OSLinuxAMD64
	if domain.CurrentOS() == foreign {
		foreign = domain.OSWindowsAMD64
	}
	arrow := preinstalledArrow(ns)
	arrow.Targets = map[domain.OS]domain.Target{foreign: arrow.Targets[domain.CurrentOS()]}

	var probed atomic.Bool
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, arrow), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				probed.Store(true)
				return nil
			},
			func(_ context.Context, _ domain.Namespace) error { return nil },
			func(_ context.Context, _ domain.Namespace) error { return nil },
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))
	assert.False(t, probed.Load())
}

// TestArrowService_Add_PreinstalledProbeVariables checks the probe is expanded
// against the facts Add can compute without an aggregate, a workdir or a port
// allocation — none of which exist for a namespace that is not in the catalog
// yet.
func TestArrowService_Add_PreinstalledProbeVariables(t *testing.T) {
	ctx := context.Background()
	ns := testNs()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(ctx) })
	axRuntime := newTestAsynxRuntime(t)

	arrow := preinstalledArrow(ns)
	arrow.Variables = []domain.Variable{
		{Name: "WITH_DEFAULT", Default: "yes"},
		{Name: "WITHOUT_DEFAULT"},
	}

	var (
		mu   sync.Mutex
		vars map[string]string
	)
	cat := arrowRepo.NewTestable(
		resolvesTo(ns, arrow), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, got map[string]string) error {
				mu.Lock()
				defer mu.Unlock()
				vars = got
				return nil
			},
			runtimeRepo.MarkPreinstalled(axRuntime),
			runtimeRepo.ForgetPreinstalled(axRuntime),
		),
	)

	require.NoError(t, cat.Add(ctx, ns, models.AddOptions{}))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, ns.String(), vars[domain.VarArrowNamespace])
	assert.Equal(t, domain.CurrentOS().String(), vars[domain.VarPlatform])
	assert.Equal(t, ns.Ref(), vars[domain.VarRef])
	assert.Equal(t, "yes", vars["WITH_DEFAULT"])
	assert.NotContains(t, vars, "WITHOUT_DEFAULT", "a variable with no default has no value to expand to")
	assert.NotContains(t, vars, domain.VarWorkdir, "no workdir exists for an arrow that is not in the catalog yet")
	assert.NotContains(t, vars, domain.VarInstallPath)
}

// TestArrowService_Add_PreinstalledCatalogLookupFails_AddsNothing: Add cannot
// tell a first announcement from a repeat one without this read, and probing on
// a wrong answer would re-run the check against a runtime that has moved on. It
// fails the add rather than guessing.
func TestArrowService_Add_PreinstalledCatalogLookupFails_AddsNothing(t *testing.T) {
	ctx := context.Background()
	ns := testNs()

	lookupErr := errors.New("event store unavailable")
	var probed atomic.Bool
	axArrow := &arrowMocks.AsynxArrow{
		ExistsFn: func(context.Context, string) (bool, error) { return false, lookupErr },
	}

	cat := arrowRepo.NewTestable(
		resolvesTo(ns, preinstalledArrow(ns)), axArrow, nil, nil,
		arrowRepo.WithPreinstalledDetection(
			domain.CurrentOS(),
			func(_ context.Context, _ domain.Namespace, _ domainStep.StepList, _ map[string]string) error {
				probed.Store(true)
				return nil
			},
			func(_ context.Context, _ domain.Namespace) error { return nil },
			func(_ context.Context, _ domain.Namespace) error { return nil },
		),
	)

	err := cat.Add(ctx, ns, models.AddOptions{})

	require.ErrorIs(t, err, lookupErr)
	assert.False(t, probed.Load(), "nothing is probed on an answer Add could not get")
}

// ─── manifold error mapping ──────────────────────────────────────────────────

// A manifest the ruleset rejects is the caller's problem, not the server's.
// Reaching the API unmapped is what turns a precise validation failure into
// "500 internal error".
func TestAdd_RulesetRejectionMapsToInvalidManifest(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(
			_ context.Context, ns domain.Namespace,
			_ string,
		) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", fmt.Errorf("reader resolve for install: %w", aerrors.RuleErrors{{
				Field:   "targets[linux/*].lifecycle.install[0].url",
				Rule:    "insufficient_coverage",
				Message: "missing coverage",
			}})
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	err := cat.Add(context.Background(), testNs(), models.AddOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidManifest)
	assert.Contains(t, err.Error(), "insufficient_coverage",
		"the rule that rejected the manifest must survive the mapping")
}

func TestAdd_NoSupportedPlatformMapsToPlatformNotSupported(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(
			_ context.Context, ns domain.Namespace,
			_ string,
		) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", fmt.Errorf("reader resolve for install: %w", ruleset.ErrNoSupportedPlatform)
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	err := cat.Add(context.Background(), testNs(), models.AddOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrPlatformNotSupported)
}

// Anything else that fails while reaching the remote is a gateway problem, not
// an internal one.
func TestAdd_RemoteFailureMapsToFetchFailed(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(
			_ context.Context, ns domain.Namespace,
			_ string,
		) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", errors.New("resolver: fetch from manifold: 503 service unavailable")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	err := cat.Add(context.Background(), testNs(), models.AddOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

// An error that already carries an app sentinel must keep it rather than being
// reclassified on the way past.
func TestAdd_ExistingSentinelIsPreserved(t *testing.T) {
	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(
			_ context.Context, ns domain.Namespace,
			_ string,
		) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", fmt.Errorf("reader resolve for install: %w", apperrors.ErrNotFound)
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)

	err := cat.Add(context.Background(), testNs(), models.AddOptions{})

	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.NotErrorIs(t, err, apperrors.ErrFetchFailed)
}
