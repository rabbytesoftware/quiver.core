package manifold

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestFletcherReleases_LatestStable(t *testing.T) {
	testCases := []struct {
		name    string
		release string
		want    string
	}{
		{name: "host release", release: "v2.0.0", want: "v2.0.0"},
		{name: "branch named release", release: "main", want: "main"},
		{name: "no stable release is a miss", want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := NewWithResolvers(
				&stubResolver{},
				&stubConstraintResolver{err: errors.New("no tags")},
				hostedBy(&stubHost{ref: tc.release}),
			).(*manifold)
			require.True(t, ok)

			tag, err := fletcherReleases{m: m}.LatestStable(context.Background(), domain.Namespace("github.com/acme/tool"))

			require.NoError(t, err)
			assert.Equal(t, tc.want, tag)
		})
	}
}

func TestFletcherReleases_LatestUnstable(t *testing.T) {
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name    string
		tags    []string
		branch  string
		listErr error
		want    string
		wantErr error
	}{
		{name: "prerelease only takes the first non-stable channel", tags: []string{"v3.0.0-beta.1", "v3.0.0-beta.2", "nightly"}, want: "v3.0.0-beta.2"},
		{name: "stable channel is skipped", tags: []string{"v2.0.0", "v3.0.0-rc1"}, want: "v3.0.0-rc1"},
		{name: "default branch fallback channel is skipped", branch: "main", want: ""},
		{name: "channel listing fails", listErr: listFailed, wantErr: listFailed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := NewWithResolvers(
				&stubResolver{},
				&stubConstraintResolver{listTags: tc.tags, listTagsErr: tc.listErr, branch: tc.branch},
				nil,
			).(*manifold)
			require.True(t, ok)

			tag, err := fletcherReleases{m: m}.LatestUnstable(context.Background(), domain.Namespace("github.com/acme/tool"))

			assert.Equal(t, tc.want, tag)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLookupFailure(t *testing.T) {
	boom := errors.New("boom")
	testCases := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "no latest stable is a miss", err: fmt.Errorf("wrap: %w", ErrNoLatestStable), want: nil},
		{name: "no tag in channel is a miss", err: fmt.Errorf("wrap: %w", ErrNoTagInChannel), want: nil},
		{name: "anything else is a failure", err: boom, want: boom},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, lookupFailure(tc.err))
		})
	}
}

func TestFletcherReleases_ResolveDefaultBranch(t *testing.T) {
	m, ok := NewWithResolvers(
		&stubResolver{},
		&stubConstraintResolver{branch: "main", branchHash: "abc"},
		nil,
	).(*manifold)
	require.True(t, ok)

	branch, hash, err := fletcherReleases{m: m}.ResolveDefaultBranch(context.Background(), domain.Namespace("github.com/acme/tool"))

	require.NoError(t, err)
	assert.Equal(t, "main", branch)
	assert.Equal(t, "abc", hash)
}
