package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

func TestNew_BuildsEveryEngine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := engine.New(ctx, engine.WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	assert.NotNil(t, c.Vault)
	assert.NotNil(t, c.Manifold)
	assert.NotNil(t, c.Wizard)
	assert.NotNil(t, c.Netbridge)
	assert.NotNil(t, c.DepTree)
}

// bitbucket.org ships without a search URL, so the provider set is smaller than
// the platform set and that difference is deliberate.
func TestNew_ProvidersExcludeFetchOnlyPlatforms(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := engine.New(ctx, engine.WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	hosts := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		hosts = append(hosts, p.Host())
	}
	assert.Equal(t, []string{"github.com", "gitlab.com"}, hosts)
}

func TestContainer_Start_DoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c, err := engine.New(ctx, engine.WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	c.Start(ctx)
}

func TestNew_UnusableHomeDir_ReturnsError(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("not a directory"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := engine.New(ctx, engine.WithHomeDir(filepath.Join(blocked, "home")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container")
}

func TestNewProviders_UnparseableTimeout_FallsBackToTheDefault(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			SearchURL:  "https://api.github.com/search/repositories?q={query}",
			SearchKind: metadata.SearchKindGitHub,
		},
	}

	providers, err := engine.NewProviders(platforms, config.Search{ProviderTimeout: "not-a-duration"})
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, "github.com", providers[0].Host())
}

func TestNewProviders_UnknownSearchKind_ReturnsError(t *testing.T) {
	platforms := metadata.Platforms{
		"example.com": {SearchURL: "https://example.com?q={query}", SearchKind: "gitea"},
	}

	_, err := engine.NewProviders(platforms, config.Search{ProviderTimeout: "10s"})
	require.Error(t, err)
}
