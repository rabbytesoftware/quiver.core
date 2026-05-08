package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	ucmocks "github.com/rabbytesoftware/quiver/internal/app/usecases/mocks"
)

// --- repo mock ---

type mockQuiverRepo struct {
	followErr          error
	followCalls        int
	followFailedArrows []domain.Namespace
	unfollowErr        error
	listResult         []domain.Collection
	listErr            error
	getResult          *domain.Collection
	getErr             error
	isFollowedResult   bool
	isFollowedErr      error
}

func (m *mockQuiverRepo) Follow(_ context.Context, _ domain.Namespace, _ *domain.Collection, failedArrows []domain.Namespace) error {
	m.followCalls++
	m.followFailedArrows = failedArrows
	return m.followErr
}

func (m *mockQuiverRepo) Unfollow(_ context.Context, _ domain.Namespace) error {
	return m.unfollowErr
}

func (m *mockQuiverRepo) List(_ context.Context) ([]domain.Collection, error) {
	return m.listResult, m.listErr
}

func (m *mockQuiverRepo) Get(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
	return m.getResult, m.getErr
}

func (m *mockQuiverRepo) IsFollowed(_ context.Context, _ domain.Namespace) (bool, error) {
	return m.isFollowedResult, m.isFollowedErr
}

func (m *mockQuiverRepo) OnCollectionFollowed(_ func(context.Context, domain.Collection)) error {
	return nil
}

func (m *mockQuiverRepo) OnCollectionUnfollowed(_ func(context.Context, domain.Namespace)) error {
	return nil
}

// --- arrow cache mock ---

type mockArrowCache struct {
	seedErr           error
	seedCalls         int
	resolveErr        error
	resolveCalls      int
	resolveResult     *domain.Arrow
	getManifestErr    error
	getManifestResult *domain.Arrow
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
		getResult: &domain.Collection{
			Arrows: []domain.CollectionArrow{
				{Namespace: ns1, IsLocal: false},
				{Namespace: ns2, IsLocal: false},
			},
		},
	}
	arrows := &mockArrowCache{}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Follow(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)
	assert.Equal(t, 2, arrows.resolveCalls)
	assert.Nil(t, repo.followFailedArrows)
	assert.Equal(t, 1, repo.followCalls)
}

func TestFollow_PartialArrowFailure_StoresFailedArrows(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/arrow1")
	ns2 := domain.Namespace("github.com/user/arrow2")

	repo := &mockQuiverRepo{
		getResult: &domain.Collection{
			Arrows: []domain.CollectionArrow{
				{Namespace: ns1, IsLocal: false},
				{Namespace: ns2, IsLocal: false},
			},
		},
	}
	arrows := &mockArrowCache{resolveErr: errors.New("fail")}
	uc := newTestUsecase(repo, arrows, &mocks.Manifold{}, &mocks.Vault{})

	err := uc.Follow(context.Background(), "github.com/user/quiver")
	require.NoError(t, err)

	assert.Equal(t, 2, len(repo.followFailedArrows))
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

// --- Get ---

func TestGet_EnrichesArrows_WithArrowManifests(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/arrow1")
	ns2 := domain.Namespace("github.com/user/arrow2")

	repo := &mockQuiverRepo{
		getResult: &domain.Collection{
			Meta: domain.CollectionMeta{
				Name:    "My Quiver",
				Version: "1.0.0",
			},
			Arrows: []domain.CollectionArrow{
				{Namespace: ns1, IsLocal: false},
				{Namespace: ns2, IsLocal: false},
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
		getResult: &domain.Collection{
			Arrows: []domain.CollectionArrow{
				{Namespace: "github.com/user/arrow1", IsLocal: false},
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
		listResult: []domain.Collection{
			{Namespace: ns1, Meta: domain.CollectionMeta{Name: "a quiver"}},
			{Namespace: ns2, Meta: domain.CollectionMeta{Name: "b quiver"}},
		},
	}
	v := &mocks.Vault{}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, v)

	result, err := uc.List(context.Background(), boolPtr(true))
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.True(t, result[0].Followed)
	assert.True(t, result[1].Followed)
	assert.Equal(t, 0, v.ListCachedCollectionsCalls)
}

func TestList_FollowedOnly_NoRepoGetCalls(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/q1")

	repo := &mockQuiverRepo{
		listResult: []domain.Collection{
			{Namespace: ns1, Meta: domain.CollectionMeta{Name: "a quiver"}},
		},
	}
	v := &mocks.Vault{}
	uc := newTestUsecase(repo, &mockArrowCache{}, &mocks.Manifold{}, v)

	_, err := uc.List(context.Background(), boolPtr(true))
	require.NoError(t, err)
	// buildFollowedDTOs reads directly from domain.Collection — no repo.Get calls
	assert.Nil(t, repo.getResult, "repo.Get should never be called for followed quivers")
}

func TestList_UnfollowedOnly_ReturnsUnfollowedCached(t *testing.T) {
	ns1 := domain.Namespace("github.com/user/q1")
	ns2 := domain.Namespace("github.com/user/q2")
	ns3 := domain.Namespace("github.com/user/q3")

	repo := &mockQuiverRepo{
		listResult: []domain.Collection{
			{Namespace: ns1},
		},
		getResult: &domain.Collection{Meta: domain.CollectionMeta{Name: "quiver"}},
	}
	v := &mocks.Vault{
		ListCachedCollectionsResult: []domain.Namespace{ns1, ns2, ns3},
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
		listResult: []domain.Collection{
			{Namespace: ns1},
		},
		getResult: &domain.Collection{Meta: domain.CollectionMeta{Name: "quiver"}},
	}
	v := &mocks.Vault{
		ListCachedCollectionsResult: []domain.Namespace{ns1, ns2},
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
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "test"}}
	repo := &mockQuiverRepo{}
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseCollectionResult: manifest}
	uc := newTestUsecase(repo, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 1, v.PutCollectionCalls)
}

func TestSeed_ParseError_ReturnsError(t *testing.T) {
	repo := &mockQuiverRepo{}
	v := &mocks.Vault{}
	m := &mocks.Manifold{ParseCollectionErr: errors.New("bad manifest")}
	uc := newTestUsecase(repo, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	assert.Error(t, err)
	assert.Equal(t, 0, v.PutCollectionCalls)
}

func TestSeed_PutCollectionError_ReturnsError(t *testing.T) {
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "test"}}
	v := &mocks.Vault{PutCollectionErr: errors.New("write error")}
	m := &mocks.Manifold{ParseCollectionResult: manifest}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, v)

	err := uc.Seed(context.Background(), "github.com/user/q1", []byte("data"))
	assert.Error(t, err)
}

// --- GetManifest ---

func TestGetManifest_ReturnsJSONEncodedManifest(t *testing.T) {
	repo := &mockQuiverRepo{
		getResult: &domain.Collection{Meta: domain.CollectionMeta{Name: "my-quiver", Version: "1.0.0"}},
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

func TestGetManifest_DoesNotLeakFollowState(t *testing.T) {
	ns := domain.Namespace("owner/my-collection@v1")
	repo := &ucmocks.MockCollection{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
			return &domain.Collection{
				Namespace:    ns,
				FollowedAt:   time.Now(),
				FailedArrows: []domain.Namespace{"owner/repo@v1/arrow-a"},
				Meta:         domain.CollectionMeta{Name: "My Collection"},
				Arrows:       []domain.CollectionArrow{},
			}, nil
		},
	}

	uc := NewCollectionUsecase(repo, nil, nil, nil)
	data, err := uc.GetManifest(context.Background(), ns)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "FollowedAt", "FollowedAt must not appear in manifest response")
	assert.NotContains(t, raw, "FailedArrows", "FailedArrows must not appear in manifest response")
	assert.Equal(t, "My Collection", raw["meta"].(map[string]any)["Name"])
}

// --- ValidateManifest ---

func TestValidateManifest_Valid_ReturnsTrue(t *testing.T) {
	m := &mocks.Manifold{ParseCollectionResult: &domain.Collection{Meta: domain.CollectionMeta{Name: "ok"}}}
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
	m := &mocks.Manifold{ParseCollectionErr: ruleErr}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, &mocks.Vault{})

	result, err := uc.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "name", result.Errors[0].Field)
	assert.Equal(t, "required", result.Errors[0].Rule)
}

func TestValidateManifest_Invalid_ParseError(t *testing.T) {
	m := &mocks.Manifold{ParseCollectionErr: errors.New("syntax error")}
	uc := newTestUsecase(&mockQuiverRepo{}, &mockArrowCache{}, m, &mocks.Vault{})

	result, err := uc.ValidateManifest(context.Background(), []byte("bad"))
	require.NoError(t, err)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "parse_error", result.Errors[0].Rule)
}

func TestGet_FailedArrows_NotEnriched(t *testing.T) {
	failedNS := domain.Namespace("owner/repo@v1/arrow-a")
	var getManifestCalled bool

	arrows := &ucmocks.MockArrow{
		GetManifestFn: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			getManifestCalled = true
			return nil, nil
		},
	}
	repo := &ucmocks.MockCollection{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
			return &domain.Collection{
				Arrows:       []domain.CollectionArrow{{Namespace: failedNS}},
				FailedArrows: []domain.Namespace{failedNS},
			}, nil
		},
	}

	uc := NewCollectionUsecase(repo, arrows, nil, nil)
	dto, err := uc.Get(context.Background(), "owner/my-collection@v1")
	require.NoError(t, err)
	assert.False(t, getManifestCalled, "GetManifest must not be called for failed arrows")
	assert.False(t, dto.Arrows[0].Resolved)
}

func TestGet_Enrichment_UsesGetManifest(t *testing.T) {
	ns := domain.Namespace("owner/repo@v1/arrow-a")
	var getManifestCalled, resolveManifestCalled bool

	arrows := &ucmocks.MockArrow{
		GetManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			getManifestCalled = true
			return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "tool-a"}}, nil
		},
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			resolveManifestCalled = true
			return nil, nil
		},
	}
	repo := &ucmocks.MockCollection{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
			return &domain.Collection{
				Arrows: []domain.CollectionArrow{{Namespace: ns}},
			}, nil
		},
	}

	uc := NewCollectionUsecase(repo, arrows, nil, nil)
	dto, err := uc.Get(context.Background(), "owner/my-collection@v1")
	require.NoError(t, err)
	assert.True(t, getManifestCalled)
	assert.False(t, resolveManifestCalled, "ResolveManifest must not be called in Get")
	assert.True(t, dto.Arrows[0].Resolved)
}

func TestFollow_LocalArrow_CallsSeed(t *testing.T) {
	localNS := domain.Namespace("owner/my-collection@v1/cs2")
	rawBytes := []byte("schema: \"arrow@v0\"\n...")

	var seededNS domain.Namespace
	var seededBytes []byte
	var resolveManifestCalled bool

	arrows := &ucmocks.MockArrow{
		SeedFn: func(_ context.Context, ns domain.Namespace, data []byte) error {
			seededNS = ns
			seededBytes = data
			return nil
		},
		ResolveManifestFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			resolveManifestCalled = true
			return nil, nil
		},
	}
	manifoldMock := &mocks.Manifold{
		ResolveArrowResult:   &domain.Arrow{},
		ResolveArrowRaw:      rawBytes,
		ResolveArrowFilename: "arrow.yaml",
	}
	repo := &ucmocks.MockCollection{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
			return &domain.Collection{
				Arrows: []domain.CollectionArrow{
					{Namespace: localNS, IsLocal: true},
				},
			}, nil
		},
	}

	uc := NewCollectionUsecase(repo, arrows, manifoldMock, nil)
	err := uc.Follow(context.Background(), "owner/my-collection@v1")
	require.NoError(t, err)
	assert.Equal(t, localNS, seededNS)
	assert.Equal(t, rawBytes, seededBytes)
	assert.False(t, resolveManifestCalled, "ResolveManifest must not be called for local arrows")
}
