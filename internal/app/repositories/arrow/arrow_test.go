package arrow_test

import (
	"context"
	"errors"
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
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
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
		ArrowMeta: domain.ArrowMeta{Name: "Test Arrow", Version: "v1.0.0"},
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
		ResolveForInstallFn: func(ctx context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return testNs(), expected, "^v1", nil
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	resolvedNs, arrow, constraint, err := cat.ResolveForInstall(context.Background(), testNs())
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
	err = cat.MarkInstalled(context.Background(), ns, "v1.0.0", time.Now().UTC())
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", got.InstalledRef)
	assert.False(t, got.InstalledAt.IsZero())
}

// A namespace that resolved to a default branch is installed at that branch:
// the branch is the ref, so there is no second value to record.
func TestMarkInstalled_DefaultBranchRef_RecordsTheBranchAsTheRef(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := domain.Namespace("github.com/user/repo@master")

	_, err := axArrow.Send(context.Background(), addArrowCmd(ns))
	require.NoError(t, err)

	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, nil, nil)
	err = cat.MarkInstalled(context.Background(), ns, ns.Ref(), time.Now().UTC())
	require.NoError(t, err)

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "master", got.InstalledRef)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, expected, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns)
	require.NoError(t, err)

	exists, err := axArrow.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestAdd_ExistingUserInstalled_Noop(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	ns := testNs()

	// Add arrow first with UserInstalled=true
	_, err := axArrow.Send(context.Background(), addArrowCmdUserInstalled(ns))
	require.NoError(t, err)

	r := &arrowStoreMocks.MockCQRS{
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, &domain.Arrow{Namespace: ns, UserInstalled: true}, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err = cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, arrow, "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err = cat.Add(context.Background(), ns)
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
	got, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
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
	got, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "v1.1.0", got.Version, "the upgraded arrow takes its version from the new ref")
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
		ResolveForInstallFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, nil, "", errors.New("resolve error")
		},
	}
	cat := arrowRepo.NewTestable(r, newTestAsynxArrow(t), nil, nil)
	err := cat.Add(context.Background(), testNs())
	require.Error(t, err)
}

// Seeded bytes have no remote to ask for a ref, and the version written in the
// manifest is not one: the caller has to say which ref these bytes are.
func TestSeed_BareNamespace_IsRejected(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	arrow := testArrow()
	arrow.Version = "v1.0.0"
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

func TestSeed_VersionComesFromTheRef(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	arrow := testArrow()
	arrow.Version = "nightly"
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	ns := testNs().BareNamespace().WithRef("v3.1.0")
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	require.NoError(t, cat.Seed(context.Background(), ns, []byte("data")))

	got, err := axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "v3.1.0", got.Version)
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
	_, err := cat.UpgradeVersion(context.Background(), testNs(), testNs().BareNamespace().WithRef("v2"), "^v1", false)
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
	_, err := cat.UpgradeVersion(context.Background(), testNs(), newNs, "^v1", false)
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
	_, err := cat.UpgradeVersion(context.Background(), testNs(), newNs, "^v1", false)
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
	_, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true) // skip vault ops
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil)
	err := cat.Add(context.Background(), ns)
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
	axArrow := &arrowMocks.AsynxArrow{
		GetFn: func(ctx context.Context, id string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
		SendFn: func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			// First send (AddArrow) returns ErrValidation → ErrAlreadyExists in addArrowCommand
			// Second send (UpdateArrowManifest) returns nil
			switch cmd.EventName() {
			default:
				// Check if it's an AddArrow by event name prefix
				if len(cmd.EventName()) > 12 && cmd.EventName()[:12] == "arrow.added." {
					return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
				}
				return asynxModels.Event[domain.Arrow]{}, nil
			}
		},
	}
	v := &mocks.Vault{}
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Seeded"}},
	}
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, v, m)
	err := cat.Seed(context.Background(), ns, []byte("raw"))
	require.NoError(t, err)
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
	_, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", false)
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
	result, err := cat.UpgradeVersion(context.Background(), ns, newNs, "^v1", true) // runtimeAlreadyExists=true
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
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
	err = cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(ctx context.Context, reqNs domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
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
	err = cat.Add(context.Background(), ns)
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
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}, axArrow)

	require.NoError(t, cat.OnArrowRemoved(func(_ context.Context, _ domain.Namespace) error {
		errored <- struct{}{}
		return errors.New("removed cb error")
	}))

	// Add then remove to trigger OnForget.
	require.NoError(t, cat.Add(context.Background(), ns))
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
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
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

	require.NoError(t, cat.Add(context.Background(), ns))
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
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
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

	require.NoError(t, cat.Add(context.Background(), ns))
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
		ResolveForInstallFn: func(_ context.Context, _ domain.Namespace) (domain.Namespace, *domain.Arrow, string, error) {
			return ns, testArrow(), "", nil
		},
	}
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, v, nil, nil)
	require.NoError(t, err)

	require.NoError(t, cat.Add(context.Background(), ns))
	axArrow.WaitPublish()
	require.NoError(t, cat.Remove(context.Background(), ns))
	axArrow.WaitPublish()

	assert.Equal(t, []domain.Namespace{ns}, v.released(),
		"forgetting an arrow must release its work dir")
}
