package advance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func cachedCopy(
	target domain.Available,
) func(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, vault.ManifestFile, bool) {
	return func(_ context.Context, _ domain.Namespace, asked domain.Available) (*domain.Arrow, vault.ManifestFile, bool) {
		if asked != target {
			return nil, vault.ManifestFile{}, false
		}
		return adoptedManifest("Cached"), vault.ManifestFile{Content: []byte("cached"), Filename: "ARROW.md", Commit: target.Commit}, true
	}
}

// Advancing onto a commit the vault already holds a build of reads no host;
// staging an update's target always does, since a retry after a checksum
// mismatch exists to see what the host serves now.
func TestManifestAtTarget_ReuseOnlyWhereTheHostNeedNotBeAsked(t *testing.T) {
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	testCases := []struct {
		name string
		act  func(a interface {
			Advance(context.Context, domain.Namespace, domain.Available) error
			RefreshToTarget(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error)
		}) error
		wantFetches int
	}{
		{
			name: "advance reuses the cached build",
			act: func(a interface {
				Advance(context.Context, domain.Namespace, domain.Available) error
				RefreshToTarget(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error)
			},
			) error {
				return a.Advance(context.Background(), rollingNs(), target)
			},
		},
		{
			name: "staging a target reads the host",
			act: func(a interface {
				Advance(context.Context, domain.Namespace, domain.Available) error
				RefreshToTarget(context.Context, domain.Namespace, domain.Available) (*domain.Arrow, error)
			},
			) error {
				_, err := a.RefreshToTarget(context.Background(), rollingNs(), target)
				return err
			},
			wantFetches: 1,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxArrow(t)
			seedSelectorRow(t, ax, rollingNs(), domain.SelectorChannel, domain.Resolved{Ref: "nightly-latest", Commit: "c1"})
			fetches := 0
			m := &mocks.Manifold{ResolveArrowAtCommitFn: func(context.Context, domain.Namespace, string, string) (*domain.Arrow, []byte, string, error) {
				fetches++
				return adoptedManifest("Fetched"), []byte("fetched"), "ARROW.md", nil
			}}
			v := &mocks.Vault{}
			cat := newTestable(&arrowStoreMocks.MockCQRS{CachedAtCommitFn: cachedCopy(target)}, ax, v, m)

			require.NoError(t, tc.act(cat))
			assert.Equal(t, tc.wantFetches, fetches)
			require.NotEmpty(t, v.PutArrowFiles)
			last := v.PutArrowFiles[len(v.PutArrowFiles)-1]
			assert.Equal(t, target, domain.Available{Ref: last.Ref, Commit: last.Commit}, "the cache records the release it holds")
		})
	}
}

func TestRefreshToTarget_AbsentTargetIsRecorded(t *testing.T) {
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	ax := newTestAsynxArrow(t)
	seedSelectorRow(t, ax, rollingNs(), domain.SelectorChannel, domain.Resolved{Ref: "nightly-latest", Commit: "c1"})
	m := &mocks.Manifold{ResolveArrowAtCommitErr: manifoldresolver.ErrNotFound}
	r := &arrowStoreMocks.MockCQRS{}
	cat := newTestable(r, ax, &mocks.Vault{}, m)

	_, err := cat.RefreshToTarget(context.Background(), rollingNs(), target)

	require.Error(t, err)
	assert.Equal(t, []domain.Available{target}, r.RecordAbsentCalls)
}
