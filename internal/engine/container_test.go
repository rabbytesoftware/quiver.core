package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// release hands the container back the way the daemon does. Closing only the
// handles the test happens to know about leaves the rest open, and an open
// SQLite file is one t.TempDir cannot remove on Windows.
func release(
	t *testing.T,
	c *Container,
) {
	t.Helper()
	t.Cleanup(func() { _ = c.Shutdown(context.Background()) })
}

func TestNew_Success_PopulatesContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	release(t, c)

	assert.NotNil(t, c.Vault)
	assert.NotNil(t, c.Manifold)
	assert.NotNil(t, c.Wizard)
	assert.NotNil(t, c.Netbridge)
	assert.NotNil(t, c.DepTree)
}

func TestNew_Success_CreatesNetbridgeDBFiles(t *testing.T) {
	home := t.TempDir()

	c, err := New(context.Background(), WithHomeDir(home))
	require.NoError(t, err)
	release(t, c)

	events, err := paths.EventsAt(home)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(events, "netbridge.db"))
	assert.FileExists(t, filepath.Join(events, "netbridge_snapshots.db"))
}

func TestNew_NetbridgeEventStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "netbridge.db"), 0o750))

	_, err = New(context.Background(), WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container:")
	assert.Contains(t, err.Error(), "eventstore:")
}

func TestNew_NetbridgeSnapshotStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "netbridge_snapshots.db"), 0o750))

	_, err = New(context.Background(), WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container:")
	assert.Contains(t, err.Error(), "snapshotstore:")
}

// The vault is the last fallible step of New for a reason: it opens a database,
// and every failure after it would owe that handle a close. A directory where
// index.db belongs fails the open with netbridge already built, which is the
// path that has to release the two handles taken before it.
func TestNew_VaultIndexUnopenable_ReturnsError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(metadata.GetVaultPathAt(home), "index.db"),
		0o750,
	))

	_, err := New(context.Background(), WithHomeDir(home))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container: vault")
}

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
	_, err := New(context.Background(), WithHomeDir(string([]byte{0})))
	assert.Error(t, err)
}

func TestContainer_Start_StartsVaultWithoutBlocking(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	release(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
}

func TestContainer_Shutdown_DrainsNetbridgeThenClosesHandles(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))

	assert.Error(t, c.Shutdown(context.Background()),
		"netbridge must report its aggregate is already drained on a second call")
}

// The vault index is a SQLite file the container opened and nothing else owns.
// POSIX unlinks an open file happily, so a leak here is invisible on macOS and
// Linux and only surfaces on Windows, where the open handle fails the removal.
// Asserting the handle is dead is what makes the ownership testable anywhere.
func TestContainer_Shutdown_ClosesTheVaultIndex(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))

	_, err = c.Vault.SearchArrows(context.Background(), vault.IndexQuery{Text: "chrom"})
	assert.ErrorIs(t, err, vault.ErrClosed,
		"the container owns the vault index and must close it on shutdown")
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	drainErr := errors.New("drain boom")
	eventsErr := errors.New("events boom")
	snapshotsErr := errors.New("snapshots boom")
	vaultErr := errors.New("vault boom")

	v := &mocks.Vault{CloseErr: vaultErr}
	c := &Container{
		Vault:              v,
		Netbridge:          &mocks.Netbridge{ShutdownErr: drainErr},
		netbridgeEvents:    &countingCloser{err: eventsErr},
		netbridgeSnapshots: &countingCloser{err: snapshotsErr},
	}

	err := c.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, drainErr)
	assert.ErrorIs(t, err, eventsErr, "a failed drain must not skip closing the event store")
	assert.ErrorIs(t, err, snapshotsErr, "a failed close must not skip the remaining handles")
	assert.ErrorIs(t, err, vaultErr, "the vault must close even when every phase before it failed")
	assert.Equal(t, 1, v.CloseCalls)
}

// Callers share one budget across sibling layers (tests/kit does), so the drain
// can arrive here with nothing left on the clock. Releasing handles is local
// work that owes that budget nothing: a context already spent must still leave
// every database closed.
func TestContainer_Shutdown_ExhaustedContext_StillReleasesEveryHandle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	v := &mocks.Vault{}
	events := &countingCloser{}
	snapshots := &countingCloser{}
	c := &Container{
		Vault:              v,
		Netbridge:          &mocks.Netbridge{},
		netbridgeEvents:    events,
		netbridgeSnapshots: snapshots,
	}

	require.NoError(t, c.Shutdown(ctx))

	assert.Equal(t, 1, events.calls)
	assert.Equal(t, 1, snapshots.calls)
	assert.Equal(t, 1, v.CloseCalls, "the vault index must close on a spent budget too")
}

type countingCloser struct {
	err   error
	calls int
}

func (c *countingCloser) Close() error {
	c.calls++
	return c.err
}

// Every platform is a provider, including bitbucket.org, which answers no
// query: dropping it would take its raw-file URLs with it and make its
// manifests unfetchable.
func TestNew_ProvidersCoverEveryPlatform(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	release(t, c)

	hosts := make([]string, 0, len(c.Providers))
	searchable := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		hosts = append(hosts, p.Host())
		if p.CanSearch() {
			searchable = append(searchable, p.Host())
		}
	}

	assert.Equal(t, []string{"bitbucket.org", "github.com", "gitlab.com"}, hosts)
	assert.Equal(t, []string{"github.com", "gitlab.com"}, searchable)
}

func TestNewProviders_UnparseableTimeout_FallsBackToTheDefault(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			Kind:      metadata.KindGitHub,
			SearchURL: "https://api.github.com/search/repositories?q={query}",
		},
	}

	providers, err := newProviders(platforms, config.Search{ProviderTimeout: "not-a-duration"})
	require.NoError(t, err)
	require.Len(t, providers, 1)
	assert.Equal(t, "github.com", providers[0].Host())
}

func TestNewProviders_UnknownKind_ReturnsError(t *testing.T) {
	platforms := metadata.Platforms{
		"example.com": {Kind: "gitea", SearchURL: "https://example.com?q={query}"},
	}

	_, err := newProviders(platforms, config.Search{ProviderTimeout: "10s"})
	require.Error(t, err)
}

// The lookup is where a provider becomes a manifold host: manifold asks by
// namespace, and the host serving that domain answers with its own URL shape.
func TestHostLookup_AnswersForTheHostServingTheNamespace(t *testing.T) {
	providers, err := newProviders(metadata.GetPlatforms(), config.Search{ProviderTimeout: "10s"})
	require.NoError(t, err)

	ns := domain.Namespace("github.com/cli/cli")
	host, ok := hostLookup(providers)(ns)
	require.True(t, ok)

	rawURL, err := host.RawFileURL(ns, "main", "ARROW.md")
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/cli/cli/main/ARROW.md", rawURL)
	assert.Equal(t, []string{"main", "master"}, host.DefaultBranches())
}

// A host with no provider is a miss, not a failure: git resolves that namespace
// without any host knowledge at all.
func TestHostLookup_UnknownHostIsAMiss(t *testing.T) {
	providers, err := newProviders(metadata.GetPlatforms(), config.Search{ProviderTimeout: "10s"})
	require.NoError(t, err)

	host, ok := hostLookup(providers)(domain.Namespace("git.example.test/u/r"))
	assert.False(t, ok)
	assert.Nil(t, host)
}

func TestHostLookup_NoProviders_MissesEveryNamespace(t *testing.T) {
	host, ok := hostLookup(nil)(domain.Namespace("github.com/u/r"))
	assert.False(t, ok)
	assert.Nil(t, host)
}
