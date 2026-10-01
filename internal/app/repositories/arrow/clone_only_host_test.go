package arrow_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const cloneOnlyBare = domain.Namespace("git.example.org/tester/multi-tool")

// cloneOnlyTarget is a host that serves each release at its tag name only,
// and the release a row installed at installedRef moves to.
func cloneOnlyTarget(
	installedRef string,
) (*mocks.CloneOnlyHost, domain.Resolved, domain.Available) {
	host := &mocks.CloneOnlyHost{
		Manifests: map[string][]byte{
			"v1.2.0": mocks.ReleaseManifest("release v1.2.0"),
			"v1.3.0": mocks.ReleaseManifest("release v1.3.0"),
		},
		Snapshot: domain.RefSnapshot{
			Tags: map[string]string{
				"v1.2.0": "c120c120c120c120c120c120c120c120c120c120",
				"v1.3.0": "c130c130c130c130c130c130c130c130c130c130",
			},
		},
	}
	commit := host.Snapshot.Tags[installedRef]
	installed := domain.Resolved{Ref: installedRef, Commit: commit, Fingerprint: commit}
	return host, installed, domain.Available{Ref: "v1.3.0", Commit: host.Snapshot.Tags["v1.3.0"]}
}

func cloneOnlySelectors() []struct {
	name string
	ns   domain.Namespace
	kind domain.SelectorKind
} {
	return []struct {
		name string
		ns   domain.Namespace
		kind domain.SelectorKind
	}{
		{name: "stable channel", ns: cloneOnlyBare.WithRef("stable"), kind: domain.SelectorChannel},
		{name: "constraint", ns: cloneOnlyBare.WithRef("v1.*"), kind: domain.SelectorConstraint},
	}
}

func newCloneOnlyCatalog(
	t *testing.T,
	host *mocks.CloneOnlyHost,
	ns domain.Namespace,
	kind domain.SelectorKind,
	installed domain.Resolved,
) (arrowRepo.Arrow, func() domain.Arrow) {
	t.Helper()
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })
	cat, err := arrowRepo.NewTestableProjecting(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, host.Manifold(), &recordingHub{})
	require.NoError(t, err)
	seedSelectorRow(t, axArrow, ns, kind, installed)

	return cat, func() domain.Arrow {
		row, err := axArrow.Get(context.Background(), ns.String())
		require.NoError(t, err)
		return row
	}
}

func TestAdvance_CloneOnlyHost_FetchesTheTargetRef(t *testing.T) {
	for _, tc := range cloneOnlySelectors() {
		t.Run(tc.name, func(t *testing.T) {
			host, installed, target := cloneOnlyTarget("v1.2.0")
			cat, row := newCloneOnlyCatalog(t, host, tc.ns, tc.kind, installed)

			require.NoError(t, cat.Advance(context.Background(), tc.ns, target))

			got := row()
			assert.Equal(t, tc.ns, got.Namespace)
			assert.Equal(t, domain.Resolved{Ref: target.Ref, Commit: target.Commit, Fingerprint: target.Commit}, got.Resolved)
			assert.Equal(t, "release v1.3.0", got.Name)
			assert.Equal(t, []domain.Namespace{
				cloneOnlyBare.WithRef(target.Commit),
				cloneOnlyBare.WithRef(target.Ref),
			}, host.Requests())
		})
	}
}

func TestRefreshToTarget_CloneOnlyHost_FetchesTheTargetRef(t *testing.T) {
	for _, tc := range cloneOnlySelectors() {
		t.Run(tc.name, func(t *testing.T) {
			host, installed, target := cloneOnlyTarget("v1.2.0")
			cat, row := newCloneOnlyCatalog(t, host, tc.ns, tc.kind, installed)

			staged, err := cat.RefreshToTarget(context.Background(), tc.ns, target)
			require.NoError(t, err)

			assert.Equal(t, tc.ns, staged.Namespace)
			assert.Equal(t, "release v1.3.0", staged.Name)
			got := row()
			assert.Equal(t, "release v1.3.0", got.Name)
			assert.Equal(t, installed, got.Resolved, "staging never moves what is installed")
			assert.Equal(t, []domain.Namespace{
				cloneOnlyBare.WithRef(target.Commit),
				cloneOnlyBare.WithRef(target.Ref),
			}, host.Requests())
		})
	}
}
