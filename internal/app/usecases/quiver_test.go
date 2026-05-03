package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/mocks"
)

// --- repo mock ---

type mockQuiverRepo struct {
	followErr            error
	followCalls          int
	unfollowErr          error
	listResult           []domain.Quiver
	listErr              error
	getResult            *domain.QuiverManifest
	getLocalBytes        map[domain.Namespace][]byte
	getErr               error
	updateFailedErr      error
	updateFailedCalls    int
	updateFailedArgs     []domain.Namespace
	isFollowedResult     bool
	isFollowedErr        error
}

func (m *mockQuiverRepo) Follow(_ context.Context, _ domain.Namespace) error {
	m.followCalls++
	return m.followErr
}

func (m *mockQuiverRepo) Unfollow(_ context.Context, _ domain.Namespace) error {
	return m.unfollowErr
}

func (m *mockQuiverRepo) List(_ context.Context) ([]domain.Quiver, error) {
	return m.listResult, m.listErr
}

func (m *mockQuiverRepo) Get(_ context.Context, _ domain.Namespace) (*domain.QuiverManifest, map[domain.Namespace][]byte, error) {
	if m.getLocalBytes == nil {
		return m.getResult, map[domain.Namespace][]byte{}, m.getErr
	}
	return m.getResult, m.getLocalBytes, m.getErr
}

func (m *mockQuiverRepo) UpdateFailedArrows(_ context.Context, _ domain.Namespace, failedArrows []domain.Namespace) error {
	m.updateFailedCalls++
	m.updateFailedArgs = failedArrows
	return m.updateFailedErr
}

func (m *mockQuiverRepo) IsFollowed(_ context.Context, _ domain.Namespace) (bool, error) {
	return m.isFollowedResult, m.isFollowedErr
}

func (m *mockQuiverRepo) OnQuiverFollowed(_ func(context.Context, domain.Quiver)) error {
	return nil
}

func (m *mockQuiverRepo) OnQuiverUnfollowed(_ func(context.Context, domain.Namespace)) error {
	return nil
}

// --- arrow cache mock ---

type mockArrowCache struct {
	seedErr             error
	seedCalls           int
	resolveErr          error
	resolveCalls        int
	resolveResult       *domain.Arrow
	getManifestErr      error
	getManifestResult   *domain.Arrow
}

func (m *mockArrowCache) Seed(_ context.Context, _ domain.Namespace, _ []byte) error {
	m.seedCalls++
	return m.seedErr
}

func (m *mockArrowCache) ResolveManifest(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	m.resolveCalls++
	return m.resolveResult, m.resolveErr
}

func (m *mockArrowCache) GetManifest(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return m.getManifestResult, m.getManifestErr
}

// --- helpers ---

func newTestUsecase(
	repo *mockQuiverRepo,
	arrows *mockArrowCache,
	manifoldMock *mocks.Manifold,
	vaultMock *mocks.Vault,
) *quiverUsecase {
	return &quiverUsecase{
		repo:     repo,
		arrows:   arrows,
		manifold: manifoldMock,
		vault:    vaultMock,
	}
}

func boolPtr(b bool) *bool { return &b }

// --- withRetry unit tests ---

func TestWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
	calls := 0
	err := withRetry(3, func() error {
		calls++
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWithRetry_RetriesUntilSuccess(t *testing.T) {
	calls := 0
	err := withRetry(2, func() error {
		calls++
		if calls < 3 {
			return errors.New("fail")
		}
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWithRetry_ExhaustsRetries(t *testing.T) {
	calls := 0
	err := withRetry(2, func() error {
		calls++
		return errors.New("fail")
	})
	assert.Error(t, err)
	assert.Equal(t, 3, calls)
}

// --- Follow ---

func TestFollow_CachesExternalArrows(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/arrow1")
	ns2 := domain.Namespace("github.com/user/arrow2")

	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{
			Arrows: []domain.QuiverArrow{
				{Namespace: ns1},
				{Namespace: ns2},
			},
		},
	}
	arrows := &mockArrowCache{}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Follow(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)
	assert.Equal(t, 2, arrows.resolveCalls)
	assert.Equal(t, 0, repo.updateFailedCalls)
	assert.Equal(t, 1, repo.followCalls)
}

func TestFollow_PartialArrowFailure_StoresFailedArrows(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/arrow1")
	ns2 := domain.Namespace("github.com/user/arrow2")

	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{
			Arrows: []domain.QuiverArrow{
				{Namespace: ns1},
				{Namespace: ns2},
			},
		},
	}
	arrows := &mockArrowCache{resolveErr: errors.New("fail")}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Follow(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)

	assert.Equal(t, 1, repo.updateFailedCalls)
	assert.Equal(t, 2, len(repo.updateFailedArgs))
	assert.Equal(t, 1, repo.followCalls)
}

func TestFollow_AutoRetry_RetriesBeforeFailure(t *testing.T) {
	// Direct retry count test: withRetry(3, always-fail) calls fn 4 times (initial + 3 retries)
	calls := 0
	err := withRetry(3, func() error {
		calls++
		return errors.New("fail")
	})
	assert.Error(t, err)
	assert.Equal(t, 4, calls)
}

func TestFollow_LocalArrows_UsesSeed(t *testing.T) {
	arrowNS := domain.Namespace("github.com/user/quiver/tool-a")
	localData := []byte("arrow manifest bytes")

	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{
			Arrows: []domain.QuiverArrow{
				{Namespace: arrowNS},
			},
		},
		getLocalBytes: map[domain.Namespace][]byte{
			arrowNS: localData,
		},
	}
	arrows := &mockArrowCache{}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Follow(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)
	assert.Equal(t, 1, arrows.seedCalls)
	assert.Equal(t, 0, arrows.resolveCalls)
}

// --- Get ---

func TestGet_EnrichesArrows_WithArrowManifests(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/arrow1")
	ns2 := domain.Namespace("github.com/user/arrow2")

	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{
			Name:    "My Quiver",
			Version: "1.0.0",
			Arrows: []domain.QuiverArrow{
				{Namespace: ns1},
				{Namespace: ns2},
			},
		},
		isFollowedResult: true,
	}
	arrows := &mockArrowCache{
		getManifestResult: &domain.Arrow{
			ArrowMeta: domain.ArrowMeta{
				Name:        "test-arrow",
				Version:     "1.2.3",
				Description: "A test arrow",
			},
		},
	}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	dto, err := uc.Get(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)
	require.NotNil(t, dto)

	assert.Equal(t, "My Quiver", dto.Name)
	assert.True(t, dto.Followed)
	assert.Len(t, dto.Arrows, 2)
	assert.True(t, dto.Arrows[0].Resolved)
	assert.Equal(t, "test-arrow", dto.Arrows[0].Name)
	assert.Equal(t, "1.2.3", dto.Arrows[0].Version)
	assert.True(t, dto.Arrows[1].Resolved)
}

func TestGet_EnrichmentFailure_ReturnsResolvedFalse(t *testing.T) {
	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{
			Arrows: []domain.QuiverArrow{
				{Namespace: "github.com/user/arrow1"},
			},
		},
	}
	arrows := &mockArrowCache{getManifestErr: errors.New("not found")}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	dto, err := uc.Get(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)
	require.NotNil(t, dto)

	assert.Len(t, dto.Arrows, 1)
	assert.False(t, dto.Arrows[0].Resolved)
}

// --- List ---

func TestList_FollowedOnly_ReturnsOnlyFollowed(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/q1")
	ns2 := domain.Namespace("github.com/user/q2")

	repo := &mockQuiverRepo{
		listResult: []domain.Quiver{
			{Namespace: ns1},
			{Namespace: ns2},
		},
		getResult: &domain.QuiverManifest{Name: "a quiver"},
	}
	v := &mocks.Vault{}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, v)

	result, err := uc.List(context.Background(), boolPtr(true))
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.True(t, result[0].Followed)
	assert.True(t, result[1].Followed)
	assert.Equal(t, 0, v.ListCachedQuiversCalls)
}

func TestList_UnfollowedOnly_ReturnsUnfollowedCached(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/q1")
	ns2 := domain.Namespace("github.com/user/q2")
	ns3 := domain.Namespace("github.com/user/q3")

	repo := &mockQuiverRepo{
		listResult: []domain.Quiver{
			{Namespace: ns1},
		},
		getResult: &domain.QuiverManifest{Name: "quiver"},
	}
	v := &mocks.Vault{
		ListCachedQuiversResult: []domain.Namespace{ns1, ns2, ns3},
	}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, v)

	result, err := uc.List(context.Background(), boolPtr(false))
	require.NoError(t, err)
	assert.Len(t, result, 2)
	for _, r := range result {
		assert.False(t, r.Followed)
		assert.NotEqual(t, ns1, r.Namespace)
	}
}

func TestList_All_ReturnsBoth(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/q1")
	ns2 := domain.Namespace("github.com/user/q2")

	repo := &mockQuiverRepo{
		listResult: []domain.Quiver{
			{Namespace: ns1},
		},
		getResult: &domain.QuiverManifest{Name: "quiver"},
	}
	v := &mocks.Vault{
		ListCachedQuiversResult: []domain.Namespace{ns1, ns2},
	}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, v)

	result, err := uc.List(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	followedCount := 0
	for _, r := range result {
		if r.Followed {
			followedCount++
		}
	}
	assert.Equal(t, 1, followedCount)
}

// --- Seed ---

func TestSeed_ParsesAndStoresManifest(t *testing.T) {
	manifest := &domain.QuiverManifest{Name: "test"}
	repo := &mockQuiverRepo{}
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseQuiverResult: manifest}
	uc := newTestUsecase(repo, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 1, v.PutQuiverCalls)
}

func TestSeed_ParseError_ReturnsError(t *testing.T) {
	repo := &mockQuiverRepo{}
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseQuiverErr: errors.New("bad manifest")}
	uc := newTestUsecase(repo, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	assert.Error(t, err)
	assert.Equal(t, 0, v.PutQuiverCalls)
}

func TestSeed_PutQuiverError_ReturnsError(t *testing.T) {
	manifest := &domain.QuiverManifest{Name: "test"}
	v := &mocks.Vault{PutQuiverErr: errors.New("write error")}
	m := &mocks.Manifold{ParseQuiverResult: manifest}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	assert.Error(t, err)
}

// --- GetManifest ---

func TestGetManifest_ReturnsJSONEncodedManifest(t *testing.T) {
	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{Name: "my-quiver", Version: "1.0.0"},
	}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, &mocks.Vault{})

	data, err := uc.GetManifest(context.Background(), "github.com/user/q1")
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Contains(t, string(data), "my-quiver")
}

func TestGetManifest_RepoGetFails_ReturnsError(t *testing.T) {
	repo := &mockQuiverRepo{getErr: errors.New("vault error")}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, &mocks.Vault{})

	_, err := uc.GetManifest(context.Background(), "github.com/user/quiver")
	require.Error(t, err)
}

// --- ValidateManifest ---

func TestValidateManifest_Valid_ReturnsTrue(t *testing.T) {
	m := &mocks.Manifold{ParseQuiverResult: &domain.QuiverManifest{Name: "ok"}}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, &mocks.Vault{})

	result, err := uc.ValidateManifest(context.Background(), []byte("any"))
	require.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidateManifest_Invalid_RuleErrors(t *testing.T) {
	ruleErr := ruleset.RuleErrors{
		{Field: "name", Rule: "required", Message: "name is required"},
	}
	m := &mocks.Manifold{ParseQuiverErr: ruleErr}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, &mocks.Vault{})

	result, err := uc.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "name", result.Errors[0].Field)
	assert.Equal(t, "required", result.Errors[0].Rule)
}

func TestValidateManifest_Invalid_ParseError(t *testing.T) {
	m := &mocks.Manifold{ParseQuiverErr: errors.New("syntax error")}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, &mocks.Vault{})

	result, err := uc.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "parse_error", result.Errors[0].Rule)
}

// --- backward compat stubs ---

func TestAdd_DelegatesToFollow(t *testing.T) {
	repo := &mockQuiverRepo{
		getResult: &domain.QuiverManifest{},
	}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Add(context.Background(), "github.com/user/q1")
	require.NoError(t, err)
	assert.Equal(t, 1, repo.followCalls)
}

func TestUpdate_ReturnsNil(t *testing.T) {
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, &mocks.Manifold{}, &mocks.Vault{})
	err := uc.Update(context.Background(), "github.com/user/q1")
	assert.NoError(t, err)
}

func TestRemove_DelegatesToUnfollow(t *testing.T) {
	repo := &mockQuiverRepo{}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, &mocks.Vault{})
	err := uc.Remove(context.Background(), "github.com/user/q1")
	assert.NoError(t, err)
}

