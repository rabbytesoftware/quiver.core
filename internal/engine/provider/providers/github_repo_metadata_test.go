package providers

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	repoAPI    = "https://api.github.com/repos/u/r"
	repoAPITpl = "https://api.github.com/repos/{user}/{repo}"
	metaBody   = `{"description":"  Finds things.  ","owner":{"avatar_url":"https://avatars.githubusercontent.com/u/1?v=4"}}`
)

var metaNS = domain.Namespace("github.com/u/r")

func metaConfig(
	doer *routedDoer,
) Config {
	cfg := githubReleaseConfig(doer)
	cfg.RepoAPIURL = repoAPITpl
	return cfg
}

func okMeta(
	body string,
) fns.Response {
	return fns.Response{Status: http.StatusOK, Body: []byte(body), Headers: http.Header{}}
}

func TestGitHub_RepoMetadata_ReadsDescriptionAndAvatar(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{repoAPI: okMeta(metaBody)}}
	provider := NewGitHub(metaConfig(doer))

	got, err := provider.RepoMetadata(context.Background(), metaNS)

	require.NoError(t, err)
	assert.Equal(t, "Finds things.", got.Description)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/1?v=4", got.AvatarURL)
	assert.Equal(t, []string{repoAPI}, doer.requests)
}

func TestGitHub_RepoMetadata_AsksOncePerRepositoryAcrossRefsAndCalls(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{repoAPI: okMeta(metaBody)}}
	provider := NewGitHub(metaConfig(doer))

	var wg sync.WaitGroup
	for _, ns := range []domain.Namespace{"github.com/u/r", "github.com/u/r@v1.0.0", "github.com/u/r@v2.0.0", "github.com/u/r@main"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := provider.RepoMetadata(context.Background(), ns)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	_, err := provider.RepoMetadata(context.Background(), metaNS)

	require.NoError(t, err)
	assert.Len(t, doer.requests, 1)
}

func TestGitHub_RepoMetadata_DifferentRepositoriesAreSeparateRequests(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		repoAPI:                                okMeta(metaBody),
		"https://api.github.com/repos/u/other": okMeta(metaBody),
	}}
	provider := NewGitHub(metaConfig(doer))

	_, errA := provider.RepoMetadata(context.Background(), metaNS)
	_, errB := provider.RepoMetadata(context.Background(), domain.Namespace("github.com/u/other"))

	require.NoError(t, errA)
	require.NoError(t, errB)
	assert.Len(t, doer.requests, 2)
}

func TestGitHub_RepoMetadata_FailuresAreMisses(t *testing.T) {
	testCases := []struct {
		name string
		do   fns.Response
		err  error
	}{
		{name: "not found", do: fns.Response{Status: http.StatusNotFound, Headers: http.Header{}}},
		{name: "rate limited", do: fns.Response{Status: http.StatusForbidden, Headers: http.Header{"X-Ratelimit-Remaining": {"0"}}}},
		{name: "server error", do: fns.Response{Status: http.StatusBadGateway, Headers: http.Header{}}},
		{name: "not json", do: okMeta("<html>")},
		{name: "network", err: errors.New("boom")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &routedDoer{responses: map[string]fns.Response{repoAPI: tc.do}}
			if tc.err != nil {
				doer.failures = map[string]error{repoAPI: tc.err}
			}
			provider := NewGitHub(metaConfig(doer))

			_, err := provider.RepoMetadata(context.Background(), metaNS)

			require.Error(t, err)
		})
	}
}

func TestGitHub_RepoMetadata_FailureIsNotRetriedUntilItExpires(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{}}
	clock := time.Unix(1_000, 0)
	cfg := metaConfig(doer)
	cfg.Now = func() time.Time { return clock }
	provider := NewGitHub(cfg)

	_, _ = provider.RepoMetadata(context.Background(), metaNS)
	_, _ = provider.RepoMetadata(context.Background(), metaNS)
	assert.Len(t, doer.requests, 1)

	clock = clock.Add(repoMetadataFailureTTL)
	doer.responses[repoAPI] = okMeta(metaBody)
	got, err := provider.RepoMetadata(context.Background(), metaNS)

	require.NoError(t, err)
	assert.Equal(t, "Finds things.", got.Description)
	assert.Len(t, doer.requests, 2)
}

func TestGitHub_RepoMetadata_WithoutAnEndpointIsAMiss(t *testing.T) {
	doer := &routedDoer{}
	provider := NewGitHub(githubReleaseConfig(doer))

	_, err := provider.RepoMetadata(context.Background(), metaNS)

	require.ErrorIs(t, err, ErrNoRepoMetadata)
	assert.Empty(t, doer.requests)
}

func TestGitHub_RepoMetadata_InvalidNamespaceIsAMiss(t *testing.T) {
	doer := &routedDoer{}
	provider := NewGitHub(metaConfig(doer))

	_, err := provider.RepoMetadata(context.Background(), domain.Namespace("nonsense"))

	require.Error(t, err)
	assert.Empty(t, doer.requests)
}

func TestGitHub_RepoMetadata_CallerCancellationDoesNotPoisonTheCache(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{repoAPI: okMeta(metaBody)}}
	provider := NewGitHub(metaConfig(doer))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = provider.RepoMetadata(ctx, metaNS)
	got, err := provider.RepoMetadata(context.Background(), metaNS)

	require.NoError(t, err)
	assert.Equal(t, "Finds things.", got.Description)
	assert.Len(t, doer.requests, 1)
}

func TestRepoMetadataCache_Get_EvictsWhenFull(t *testing.T) {
	cache := newRepoMetadataCache(time.Now)
	load := func(context.Context) (domain.RepoMetadata, error) { return domain.RepoMetadata{}, nil }

	for i := range repoMetadataMaxEntries + 1 {
		_, err := cache.get(context.Background(), string(rune('a'+i%26))+time.Duration(i).String(), load)
		require.NoError(t, err)
	}

	assert.LessOrEqual(t, len(cache.entries), repoMetadataMaxEntries)
}

func TestHost_RepoMetadata_PlainHostOffersNone(t *testing.T) {
	provider := NewGitLab(Config{Host: "gitlab.com"})

	_, err := provider.RepoMetadata(context.Background(), domain.Namespace("gitlab.com/u/r"))

	require.ErrorIs(t, err, ErrNoRepoMetadata)
}
