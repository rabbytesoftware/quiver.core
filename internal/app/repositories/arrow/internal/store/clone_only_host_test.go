package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const cloneOnlyBare = domain.Namespace("git.example.org/tester/multi-tool")

// cloneOnlyHost serves each release at its tag name and nothing else: neither
// a commit SHA nor a selector such as "stable" or "v1.*" names a git ref.
func cloneOnlyHost() *mocks.CloneOnlyHost {
	return &mocks.CloneOnlyHost{
		Manifests: map[string][]byte{
			"v1.2.0": mocks.ReleaseManifest("release v1.2.0"),
			"v1.3.0": mocks.ReleaseManifest("release v1.3.0"),
			"v2.0.0": mocks.ReleaseManifest("release v2.0.0"),
		},
		Snapshot: domain.RefSnapshot{
			Tags: map[string]string{
				"v1.2.0": "c120c120c120c120c120c120c120c120c120c120",
				"v1.3.0": "c130c130c130c130c130c130c130c130c130c130",
				"v2.0.0": "c200c200c200c200c200c200c200c200c200c200",
			},
			Branches: map[string]string{"main": "c200c200c200c200c200c200c200c200c200c200"},
			Head:     "main",
		},
	}
}

func newCloneOnlyReader(
	t *testing.T,
	host *mocks.CloneOnlyHost,
) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, &mocks.Vault{}, host.Manifold())
	require.NoError(t, err)
	return r
}

func TestResolveInstall_CloneOnlyHost_FetchesTheResolvedRef(t *testing.T) {
	testCases := []struct {
		name         string
		selector     string
		wantKind     domain.SelectorKind
		wantResolved string
	}{
		{name: "stable channel", selector: "stable", wantKind: domain.SelectorOrderedChannel, wantResolved: "v2.0.0"},
		{name: "refless follows the default channel", selector: "", wantKind: domain.SelectorOrderedChannel, wantResolved: "v2.0.0"},
		{name: "constraint", selector: "v1.*", wantKind: domain.SelectorConstraint, wantResolved: "v1.3.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := cloneOnlyHost()
			r := newCloneOnlyReader(t, host)

			identity, arrow, err := r.ResolveInstall(context.Background(), cloneOnlyBare.WithRef(tc.selector))
			require.NoError(t, err)

			commit := host.Snapshot.Tags[tc.wantResolved]
			assert.Equal(t, identity, arrow.Namespace)
			assert.Equal(t, tc.wantKind, arrow.SelectorKind)
			assert.Equal(t, tc.wantResolved, arrow.Resolved.Ref)
			assert.Equal(t, commit, arrow.Resolved.Commit)
			assert.Equal(t, "release "+tc.wantResolved, arrow.Name)
			assert.Equal(t, []domain.Namespace{
				cloneOnlyBare.WithRef(commit),
				cloneOnlyBare.WithRef(tc.wantResolved),
			}, host.Requests())
		})
	}
}

func TestResolveAdoption_CloneOnlyHost_FetchesTheAdmittedRef(t *testing.T) {
	testCases := []struct {
		name     string
		selector string
		ref      string
	}{
		{name: "stable channel adopts an older member", selector: "stable", ref: "v1.2.0"},
		{name: "constraint adopts a match below its highest", selector: "v1.*", ref: "v1.2.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := cloneOnlyHost()
			r := newCloneOnlyReader(t, host)
			identity := cloneOnlyBare.WithRef(tc.selector)

			adoption, err := r.ResolveAdoption(context.Background(), identity, tc.ref)
			require.NoError(t, err)

			commit := host.Snapshot.Tags[tc.ref]
			assert.Equal(t, identity, adoption.Identity)
			assert.Equal(t, domain.Resolved{Ref: tc.ref, Commit: commit, Fingerprint: commit}, adoption.Resolved)
			assert.Equal(t, host.Manifests[tc.ref], adoption.Manifest)
			assert.Equal(t, []domain.Namespace{
				cloneOnlyBare.WithRef(commit),
				cloneOnlyBare.WithRef(tc.ref),
			}, host.Requests())
		})
	}
}
