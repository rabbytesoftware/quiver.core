package manifold

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const inferredArrow = "A tool that does things.\n\n" +
	"```arrow\n" +
	`schema: arrow@v0
metadata:
  name: tool
  description: A tool that does things.
  url: https://github.com/acme/tool
  generator:
    name: fletcher/1
    confidence: medium
targets:
  linux/amd64:
    lifecycle:
      install:
        - type: fetch
          title: Download tool-linux-x86_64.tar.gz
          url: https://github.com/acme/tool/releases/download/v1/tool-linux-x86_64.tar.gz
          to: ${INSTALL_PATH}/.tool.download
          checksum: sha256:0000000000000000000000000000000000000000000000000000000000000001
          timeout: 15m
        - type: portable
          title: Install tool-linux-x86_64.tar.gz
          from: ${INSTALL_PATH}/.tool.download
          to: ${INSTALL_PATH}/tool
          name: tool
          timeout: 15m
    expose:
      cli:
        - name: tool
          path: auto
` + "```\n"

type stubFletcher struct {
	raw    []byte
	ref    string
	err    error
	causes []error
}

func (s *stubFletcher) Recover(
	_ context.Context,
	_ domain.Namespace,
	cause error,
) ([]byte, string, string, error) {
	s.causes = append(s.causes, cause)
	if s.err != nil {
		return nil, "", "", s.err
	}
	return s.raw, "ARROW.md", s.ref, nil
}

func withStubFletcher(
	t *testing.T,
	m Manifold,
	fl *stubFletcher,
) Manifold {
	t.Helper()
	built, ok := m.(*manifold)
	require.True(t, ok)
	built.fl = fl
	return built
}

func manifestMissing() error {
	return fmt.Errorf("%w: github.com/acme/tool@v1.2.0", resolver.ErrManifestNotFound)
}

func TestWithFletcher_ReachesEveryConstructor(t *testing.T) {
	testCases := []struct {
		name  string
		build func(opts ...Option) Manifold
	}{
		{
			name: "New",
			build: func(opts ...Option) Manifold {
				return New(time.Second, nil, time.Hour, opts...)
			},
		},
		{
			name: "NewWithResolversAndClock",
			build: func(opts ...Option) Manifold {
				return NewWithResolversAndClock(&stubResolver{}, &stubConstraintResolver{}, nil, time.Now, opts...)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enabled, ok := tc.build(WithFletcher(true)).(*manifold)
			require.True(t, ok)
			disabled, ok := tc.build(WithFletcher(false)).(*manifold)
			require.True(t, ok)
			overridden, ok := tc.build(WithFletcher(true), WithFletcher(false)).(*manifold)
			require.True(t, ok)

			assert.NotNil(t, enabled.fl)
			assert.Nil(t, disabled.fl)
			assert.Nil(t, overridden.fl)
		})
	}
}

func TestWithFletcher_ResolveArrow_FallsBackOnlyWhenEnabled(t *testing.T) {
	testCases := []struct {
		name       string
		enabled    bool
		wantReason fletcher.Reason
	}{
		{name: "disabled", enabled: false},
		{name: "enabled", enabled: true, wantReason: fletcher.ReasonHostUnsupported},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil, WithFletcher(tc.enabled))

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("example.org/acme/tool@v1.0.0"))

			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
			if tc.wantReason == "" {
				assert.ErrorIs(t, err, resolver.ErrNotFound)
				assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
				return
			}
			var nf fletcher.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestResolveArrow_FletcherEnabled_DeclaredManifestWins(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow)}
	declared := []byte(inferredArrow)
	m := withStubFletcher(t, NewWithResolvers(
		&stubResolver{arrowData: declared, arrowFilename: "arrow.yaml"},
		&stubConstraintResolver{},
		nil,
	), fl)

	_, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, declared, raw)
	assert.Equal(t, "arrow.yaml", filename)
	assert.Empty(t, fl.causes)
}

func TestResolveArrow_FletcherEnabled_HandsEveryResolveFailureToFletcher(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{name: "manifest not found", err: manifestMissing()},
		{name: "transport failure", err: fmt.Errorf("%w: HTTP 503", resolver.ErrFetchFailed)},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{err: tc.err}
			m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: tc.err}, &stubConstraintResolver{}, nil), fl)

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

			assert.ErrorIs(t, err, tc.err)
			require.Len(t, fl.causes, 1)
			assert.ErrorIs(t, fl.causes[0], tc.err)
		})
	}
}

func TestResolveArrow_FletcherEnabled_ManifestNotFound_ForgesInferredArrow(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow)}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	arrow, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, []byte(inferredArrow), raw)
	assert.Equal(t, "ARROW.md", filename)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, "tool", arrow.Name)
}

func TestResolveArrow_FletcherEnabled_ForgedBytesThatDoNotParse(t *testing.T) {
	fl := &stubFletcher{raw: []byte("not: [a manifest")}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@main"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidManifest)
	assert.NotErrorIs(t, err, resolver.ErrNotFound)
	assert.NotErrorIs(t, err, resolver.ErrFetchFailed)
}

type refFletcher struct {
	draftable string
	// draftedFrom, when set, is the release the draft really came from.
	draftedFrom string
	asked       []domain.Namespace
}

func (f *refFletcher) Recover(
	_ context.Context,
	ns domain.Namespace,
	cause error,
) ([]byte, string, string, error) {
	f.asked = append(f.asked, ns)
	if ns.Ref() != f.draftable {
		return nil, "", "", cause
	}
	if f.draftedFrom != "" {
		return []byte(inferredArrow), "ARROW.md", f.draftedFrom, nil
	}
	return []byte(inferredArrow), "ARROW.md", ns.Ref(), nil
}

// A branch of a repository with no manifest has no release of its own: a
// draft of the latest release is not what the branch holds, so a row
// following the branch cannot be built from it.
func TestResolveArrowAtCommit_DraftOfAnotherReleaseIsNoManifestAtTheRef(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	identity := domain.Namespace("github.com/acme/tool@main")
	fl := &refFletcher{draftable: "main", draftedFrom: "v1.2.0"}
	m, ok := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil).(*manifold)
	require.True(t, ok)
	m.fl = fl

	_, _, _, err := m.ResolveArrowAtCommit(context.Background(), identity, "main", commit)

	require.ErrorIs(t, err, resolver.ErrManifestNotFound)
	var nf fletcher.NotFletchableError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, fletcher.ReasonNotARelease, nf.Reason)
}

// A release is published under its tag, never under the commit the tag
// resolves to, so a manifest Fletcher drafts is found at the fallback ref.
func TestResolveArrowAtCommit_FletcherDraftsAtTheRefNotTheCommit(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	identity := domain.Namespace("github.com/acme/tool@stable")
	fl := &refFletcher{draftable: "v1.2.0"}
	m, ok := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil).(*manifold)
	require.True(t, ok)
	m.fl = fl

	arrow, raw, filename, err := m.ResolveArrowAtCommit(context.Background(), identity, "v1.2.0", commit)

	require.NoError(t, err)
	assert.Equal(t, identity, arrow.Namespace)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, []byte(inferredArrow), raw)
	assert.Equal(t, "ARROW.md", filename)
	assert.Equal(t, []domain.Namespace{identity.WithRef(commit), identity.WithRef("v1.2.0")}, fl.asked)
}

func TestSnapshotReleases(t *testing.T) {
	snap := domain.RefSnapshot{
		Tags:     map[string]string{"v1.0.0": "c100", "v2.0.0": "c200", "v3.0.0-rc1": "c3rc"},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
	boom := errors.New("ls-remote failed")

	testCases := []struct {
		name       string
		snap       domain.RefSnapshot
		snapErr    error
		wantStable string
		stableErr  error
		wantBranch string
		wantHash   string
		branchErr  error
	}{
		{name: "tagged repository", snap: snap, wantStable: "v2.0.0", wantBranch: "main", wantHash: "cmain"},
		{
			name:       "no stable release",
			snap:       domain.RefSnapshot{Tags: map[string]string{"v3.0.0-rc1": "c3rc"}, Branches: map[string]string{"main": "cmain"}, Head: "main"},
			stableErr:  models.ErrNoLatestStable,
			wantBranch: "main",
			wantHash:   "cmain",
		},
		{
			name:       "no head branch",
			snap:       domain.RefSnapshot{Tags: map[string]string{"v1.0.0": "c100"}},
			wantStable: "v1.0.0",
			branchErr:  ErrUnknownSelector,
		},
		{name: "unreachable remote", snapErr: boom, stableErr: boom, branchErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := NewWithResolvers(&stubResolver{}, &stubConstraintResolver{refs: &tc.snap, refsErr: tc.snapErr}, nil).(*manifold)
			require.True(t, ok)
			releases := m
			ns := domain.Namespace("github.com/acme/tool")

			stable, err := releases.ResolveLatestStable(context.Background(), ns)
			if tc.stableErr != nil {
				require.ErrorIs(t, err, tc.stableErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantStable, stable)
			}

			branch, hash, err := releases.ResolveDefaultBranch(context.Background(), ns)
			if tc.branchErr != nil {
				require.ErrorIs(t, err, tc.branchErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantBranch, branch)
				assert.Equal(t, tc.wantHash, hash)
			}

			channels, err := releases.ListChannels(context.Background(), ns)
			if tc.snapErr != nil {
				require.ErrorIs(t, err, tc.snapErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, ChannelsOf(tc.snap), channels)
		})
	}
}

func TestResolveArrow_FletcherDraftedFromAnotherRef_ArrowNamesThatRef(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow), ref: "v1.2.0"}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	arrow, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@main"))

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/acme/tool@v1.2.0"), arrow.Namespace)
}

func TestResolveArrow_FletcherEnabled_WithoutARefLeavesTheNamespaceUnset(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow)}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	arrow, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Empty(t, arrow.Namespace)
}
