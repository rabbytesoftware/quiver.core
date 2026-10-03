package assemblerinternal_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	assemblerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

const releaseNs = domain.Namespace("github.com/rabbytesoftware/quiver.core@nightly-latest")

func TestReleaseResolver_Resolve_AnswersEachSourceFromOneLookup(t *testing.T) {
	lookups := 0
	resolver := assemblerinternal.NewReleaseResolver(func(_ context.Context, ns domain.Namespace, os domain.OS) (domain.ReleaseAsset, error) {
		lookups++
		assert.Equal(t, releaseNs, ns)
		assert.Equal(t, domain.OSDarwinARM64, os)
		return domain.ReleaseAsset{Name: "quiver-darwin-arm64", URL: "https://example.test/quiver", Digest: "sha256:ABC123"}, nil
	})

	got, err := resolver.Resolve(
		context.Background(),
		releaseNs,
		domain.OSDarwinARM64,
		[]string{domain.VarSourceReleaseAsset, domain.VarSourceReleaseChecksum},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, lookups)
	assert.Equal(t, map[string]string{
		domain.VarSourceReleaseAsset:    "https://example.test/quiver",
		domain.VarSourceReleaseChecksum: "ABC123",
	}, got)
}

func TestReleaseResolver_Resolve_UnknownSource(t *testing.T) {
	resolver := assemblerinternal.NewReleaseResolver(func(context.Context, domain.Namespace, domain.OS) (domain.ReleaseAsset, error) {
		return domain.ReleaseAsset{URL: "u", Digest: "sha256:aa"}, nil
	})

	_, err := resolver.Resolve(context.Background(), releaseNs, domain.OSLinuxAMD64, []string{"release.size"})

	require.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}

func TestReleaseResolver_Resolve_ClassifiesFailures(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want apperrors.ReleaseKind
	}{
		{"rate limited", fmt.Errorf("host: %w", &provider.RateLimitedError{Host: "github.com", RetryAfter: time.Minute}), apperrors.ReleaseRateLimited},
		{"network failure", &net.OpError{Op: "dial", Err: errors.New("no route to host")}, apperrors.ReleaseOffline},
		{"any other host failure", errors.New("http 500"), apperrors.ReleaseOffline},
		{"no release", fmt.Errorf("x: %w", manifold.ErrNoRelease), apperrors.ReleaseNoRelease},
		{"no asset", fmt.Errorf("x: %w", manifold.ErrNoAsset), apperrors.ReleaseNoAsset},
		{"unsupported platform", fmt.Errorf("x: %w", manifold.ErrUnsupportedPlatform), apperrors.ReleaseUnsupportedPlatform},
		{"unverifiable", fmt.Errorf("x: %w", manifold.ErrUnverifiable), apperrors.ReleaseUnverifiable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := assemblerinternal.NewReleaseResolver(func(context.Context, domain.Namespace, domain.OS) (domain.ReleaseAsset, error) {
				return domain.ReleaseAsset{}, tc.err
			})

			_, err := resolver.Resolve(context.Background(), releaseNs, domain.OSLinuxAMD64, []string{domain.VarSourceReleaseAsset})

			require.ErrorIs(t, err, apperrors.ErrReleaseUnresolved)
			var release *apperrors.ReleaseError
			require.ErrorAs(t, err, &release)
			assert.Equal(t, tc.want, release.Kind)
			assert.ErrorIs(t, err, tc.err)
		})
	}
}

func TestReleaseResolver_Resolve_CancellationIsNotAReleaseFailure(t *testing.T) {
	resolver := assemblerinternal.NewReleaseResolver(func(context.Context, domain.Namespace, domain.OS) (domain.ReleaseAsset, error) {
		return domain.ReleaseAsset{}, fmt.Errorf("fetch: %w", context.Canceled)
	})

	_, err := resolver.Resolve(context.Background(), releaseNs, domain.OSLinuxAMD64, []string{domain.VarSourceReleaseAsset})

	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, apperrors.ErrReleaseUnresolved)
}

type stubReleases struct {
	values  map[string]string
	err     error
	calls   int
	gotNs   domain.Namespace
	gotOS   domain.OS
	sources []string
}

func (s *stubReleases) Resolve(
	_ context.Context,
	ns domain.Namespace,
	os domain.OS,
	sources []string,
) (map[string]string, error) {
	s.calls++
	s.gotNs, s.gotOS, s.sources = ns, os, sources
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}

func releaseBoundArrow(ns domain.Namespace) *domain.Arrow {
	return &domain.Arrow{
		Namespace: ns,
		Variables: []domain.Variable{
			{Name: "ASSET_URL", From: domain.VarSourceReleaseAsset},
			{Name: "CHECKSUM", From: domain.VarSourceReleaseChecksum},
			{Name: "PLAIN", Default: "x"},
		},
	}
}

func resolveWithReleases(
	t *testing.T,
	arrow *domain.Arrow,
	userVars map[string]string,
	steps []domainStep.Step,
	releases assemblerinternal.ReleaseResolver,
) (map[string]string, error) {
	t.Helper()
	return assemblerinternal.ResolveVariables(
		context.Background(),
		arrow.Namespace,
		arrow,
		domain.Target{},
		domain.OSDarwinARM64,
		testGetArrow(arrow),
		newTestAsynxRuntimeForVars(t),
		nil,
		nil,
		userVars,
		steps,
		assemblerinternal.WithReleases(releases, releaseNs),
	)
}

func TestResolveVariables_ReleaseBound_FilledFromTheRelease(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	releases := &stubReleases{values: map[string]string{
		domain.VarSourceReleaseAsset:    "https://example.test/quiver",
		domain.VarSourceReleaseChecksum: "abc",
	}}

	vars, err := resolveWithReleases(t, arrow, nil, stepsUnderTest(arrow), releases)

	require.NoError(t, err)
	assert.Equal(t, "https://example.test/quiver", vars["ASSET_URL"])
	assert.Equal(t, "abc", vars["CHECKSUM"])
	assert.Equal(t, 1, releases.calls)
	assert.Equal(t, releaseNs, releases.gotNs)
	assert.Equal(t, domain.OSDarwinARM64, releases.gotOS)
	assert.ElementsMatch(t, []string{domain.VarSourceReleaseAsset, domain.VarSourceReleaseChecksum}, releases.sources)
}

func TestResolveVariables_ReleaseBound_CallerValueWins(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	releases := &stubReleases{values: map[string]string{
		domain.VarSourceReleaseAsset:    "https://resolved.test/quiver",
		domain.VarSourceReleaseChecksum: "resolved",
	}}

	vars, err := resolveWithReleases(t, arrow,
		map[string]string{"ASSET_URL": "https://mine.test/quiver"},
		stepsUnderTest(arrow), releases)

	require.NoError(t, err)
	assert.Equal(t, "https://mine.test/quiver", vars["ASSET_URL"])
	assert.Equal(t, "resolved", vars["CHECKSUM"])
	assert.Equal(t, []string{domain.VarSourceReleaseChecksum}, releases.sources, "only the missing one is asked for")
}

func TestResolveVariables_ReleaseBound_NothingMissingMeansNoLookup(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	releases := &stubReleases{err: errors.New("must not be asked")}

	vars, err := resolveWithReleases(t, arrow,
		map[string]string{"ASSET_URL": "https://mine.test/quiver", "CHECKSUM": "mine"},
		stepsUnderTest(arrow), releases)

	require.NoError(t, err)
	assert.Equal(t, "mine", vars["CHECKSUM"])
	assert.Zero(t, releases.calls)
}

func TestResolveVariables_ReleaseBound_UnreferencedMeansNoLookup(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	releases := &stubReleases{err: errors.New("must not be asked")}
	steps := []domainStep.Step{domainStep.NewRunStep("uninstall", "echo bye", false, "10s", true)}

	vars, err := resolveWithReleases(t, arrow, nil, steps, releases)

	require.NoError(t, err)
	assert.NotContains(t, vars, "ASSET_URL")
	assert.Zero(t, releases.calls)
}

func TestResolveVariables_ReleaseBound_FailureNamesTheVariables(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	cause := apperrors.NewReleaseError(apperrors.ReleaseRateLimited, errors.New("slow down"))

	_, err := resolveWithReleases(t, arrow, nil, stepsUnderTest(arrow), &stubReleases{err: cause})

	require.ErrorIs(t, err, apperrors.ErrReleaseUnresolved)
	assert.Contains(t, err.Error(), "ASSET_URL")
	assert.Contains(t, err.Error(), "CHECKSUM")
}

func TestResolveVariables_ReleaseBound_ResolvedEmptyIsMissing(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())
	releases := &stubReleases{values: map[string]string{domain.VarSourceReleaseAsset: "https://example.test/q"}}

	_, err := resolveWithReleases(t, arrow, nil, stepsUnderTest(arrow), releases)

	require.ErrorIs(t, err, apperrors.ErrMissingVariable)
	assert.Contains(t, err.Error(), "CHECKSUM")
}

func TestResolveVariables_ReleaseBound_WithoutAResolverIsMissing(t *testing.T) {
	arrow := releaseBoundArrow(testNsForVars())

	_, err := assemblerinternal.ResolveVariables(
		context.Background(), arrow.Namespace, arrow, domain.Target{}, domain.OSDarwinARM64,
		testGetArrow(arrow), newTestAsynxRuntimeForVars(t), nil, nil, nil, stepsUnderTest(arrow),
	)

	require.ErrorIs(t, err, apperrors.ErrMissingVariable)
}
