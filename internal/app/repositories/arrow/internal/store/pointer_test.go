package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

const rollingNs = domain.Namespace("github.com/char2cs/crowbar@nightly")

func rollingManifold(commit string) *mocks.Manifold {
	return &mocks.Manifold{
		ResolveLatestInChannelRef: "nightly",
		ResolveRefCommitHash:      commit,
		ListChannelsResult: []manifold.ChannelInfo{
			{Name: "nightly", Kind: "pointer", Latest: "nightly"},
			{Name: manifold.StableChannel, Kind: "ordered", Latest: "v1.0.0"},
		},
	}
}

func rollingArrow(commit string) domain.Arrow {
	return domain.Arrow{Namespace: rollingNs, Channel: "nightly", RefCommitSHA: commit}
}

func TestCheckVersionDrift_PointerTag_SameCommit_NotOutdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, rollingManifold("aaa111"))

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), rollingArrow("aaa111"))
	require.True(t, ok)
	assert.False(t, outdated)
	assert.Empty(t, recommendedRef)
}

func TestCheckVersionDrift_PointerTag_MovedCommit_RecommendsTheSameTag(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, rollingManifold("bbb222"))

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), rollingArrow("aaa111"))
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "nightly", recommendedRef)
}

func TestCheckVersionDrift_PointerTag_NoRecordedCommit_CannotProveCurrent(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, rollingManifold("aaa111"))

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), rollingArrow(""))
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "nightly", recommendedRef)
}

func TestCheckVersionDrift_PointerTag_ChannelNeverRecorded_StillTracksTheTag(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, rollingManifold("bbb222"))
	arrow := rollingArrow("aaa111")
	arrow.Channel = ""

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "nightly", recommendedRef)
}

func TestCheckVersionDrift_PointerTag_NoChannelAndSameCommit_NotOutdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, rollingManifold("aaa111"))
	arrow := rollingArrow("aaa111")
	arrow.Channel = ""

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.False(t, outdated)
	assert.Empty(t, recommendedRef)
}

func TestCheckVersionDrift_PointerTag_CommitLookupFails_WritesNoAnswer(t *testing.T) {
	m := rollingManifold("")
	m.ResolveRefCommitErr = errors.New("dial tcp: connection refused")
	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, ok := r.CheckVersionDrift(context.Background(), rollingArrow("aaa111"))
	assert.False(t, ok)
}

func TestCheckVersionDrift_OrderedChannel_StillComparesRefNames(t *testing.T) {
	m := rollingManifold("aaa111")
	m.ResolveLatestInChannelRef = "v1.1.0"
	r := newTestReaderWithVaultManifold(t, nil, m)
	arrow := domain.Arrow{
		Namespace: domain.Namespace("github.com/char2cs/crowbar@v1.0.0"),
		Channel:   manifold.StableChannel,
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.1.0", recommendedRef)
	assert.Zero(t, m.ResolveRefCommitCalls, "an ordered tag never moves, so its commit is never asked for")
}

func TestResolveForInstall_ExplicitPointerTag_TracksItAsItsOwnChannelAndRecordsItsCommit(t *testing.T) {
	m, _ := branchServingManifold("nightly")
	m.ListChannelsResult = rollingManifold("").ListChannelsResult
	m.ResolveRefCommitHash = "aaa111"
	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(context.Background(), rollingNs, "")
	require.NoError(t, err)
	assert.Equal(t, rollingNs, resolvedNs)
	assert.Equal(t, "nightly", got.Channel)
	assert.Equal(t, "aaa111", got.RefCommitSHA)
	assert.False(t, got.RefIsBranch)
}

func TestResolveForInstall_ExplicitOrderedTag_RecordsNoCommit(t *testing.T) {
	m, _ := branchServingManifold("v1.0.0")
	m.ListChannelsResult = rollingManifold("").ListChannelsResult
	m.ResolveRefCommitHash = "aaa111"
	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(), domain.Namespace("github.com/char2cs/crowbar@v1.0.0"), "",
	)
	require.NoError(t, err)
	assert.Empty(t, got.RefCommitSHA)
	assert.Zero(t, m.ResolveRefCommitCalls)
}

func TestResolveManifest_PointerTag_RecordsTheCommitItResolvedAt(t *testing.T) {
	m := rollingManifold("bbb222")
	m.ParseArrowResult = &domain.Arrow{Namespace: rollingNs}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	r := newTestReaderWithVaultManifold(t, v, m)

	got, err := r.ResolveManifest(context.Background(), rollingNs)
	require.NoError(t, err)
	assert.Equal(t, "bbb222", got.RefCommitSHA)
}

func TestPointerCommit(t *testing.T) {
	commitErr := errors.New("dial tcp: connection refused")
	testCases := []struct {
		name string
		ns   domain.Namespace
		m    *mocks.Manifold
		want string
	}{
		{name: "pointer tag", ns: rollingNs, m: rollingManifold("aaa111"), want: "aaa111"},
		{name: "refless namespace", ns: "github.com/char2cs/crowbar", m: rollingManifold("aaa111"), want: ""},
		{name: "ordered tag", ns: "github.com/char2cs/crowbar@v1.0.0", m: rollingManifold("aaa111"), want: ""},
		{
			name: "channels cannot be listed",
			ns:   rollingNs,
			m:    &mocks.Manifold{ListChannelsErr: commitErr, ResolveRefCommitHash: "aaa111"},
			want: "",
		},
		{
			name: "default branch fallback is not a pointer tag",
			ns:   "github.com/char2cs/crowbar@develop",
			m: &mocks.Manifold{
				ListChannelsResult: []manifold.ChannelInfo{
					{Name: "develop", Kind: "pointer", Latest: "develop", IsDefaultBranchFallback: true},
				},
				ResolveRefCommitHash: "aaa111",
			},
			want: "",
		},
		{
			name: "commit cannot be read",
			ns:   rollingNs,
			m:    func() *mocks.Manifold { m := rollingManifold(""); m.ResolveRefCommitErr = commitErr; return m }(),
			want: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestReaderWithVaultManifold(t, nil, tc.m)

			assert.Equal(t, tc.want, r.PointerCommit(context.Background(), tc.ns))
		})
	}
}
