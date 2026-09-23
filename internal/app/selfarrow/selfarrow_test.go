package selfarrow_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// binaryName mirrors selfarrow's own unexported helper so the tests can
// predict the promoted filename without depending on selfarrow's internals.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "quiver.exe"
	}
	return "quiver"
}

func TestEnsureRegistered_AddsWhenAbsent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
	}
	var seededNS domain.Namespace
	m.SeedFn = func(_ context.Context, ns domain.Namespace, _ []byte) error {
		seededNS = ns
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/rabbytesoftware/quiver.core@stable-25.9.2"), seededNS)
}

// TestEnsureRegistered_ChannelConfigured_StampsChannel covers a fresh
// (not-yet-registered) self-arrow booted with a non-empty configured
// channel: EnsureRegistered must stamp it onto the newly seeded row via
// SetChannel, in addition to seeding it.
func TestEnsureRegistered_ChannelConfigured_StampsChannel(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
	}
	m.SeedFn = func(context.Context, domain.Namespace, []byte) error {
		return nil
	}
	var capturedChannel string
	m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
		capturedChannel = channel
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.NoError(t, err)
	assert.Equal(t, "rc", capturedChannel)
}

// TestEnsureRegistered_ChannelStamped_TriggersImmediateVersionCheck guards
// the regression a review caught right after resolveChannel started
// inferring a channel for every unconfigured build of a known non-stable
// kind (nightly, beta, hotfix), not just an operator's explicit
// self_update_channel: SetChannel unconditionally clears Outdated and
// RecommendedRef (see its own EmitEvent doc comment), and that clear now
// runs on every boot of the common unconfigured-nightly case, not just a
// one-time opt-in. Without an eager CheckVersionNow right after, that
// clear would stay stale until some unrelated caller happens to query this
// arrow's detail -- CheckVersionNow closes that window immediately, the
// same way switchChannel (usecases/arrow.go) already pairs the two calls.
func TestEnsureRegistered_ChannelStamped_TriggersImmediateVersionCheck(t *testing.T) {
	var checkedNs domain.Namespace
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn: func(context.Context, domain.Namespace, []byte) error {
			return nil
		},
		SetChannelFn: func(context.Context, domain.Namespace, string, string) error {
			return nil
		},
		CheckVersionNowFn: func(_ context.Context, ns domain.Namespace) {
			checkedNs = ns
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "nightly-a1b2c3d", "")

	require.NoError(t, err)
	assert.NotEmpty(t, checkedNs, "expected CheckVersionNow to fire for the freshly channel-stamped self-arrow row")
}

// TestEnsureRegistered_ChannelInference_FallsBackToVersionPrefix covers the
// real bug this fixes: with no self_update_channel configured, a version
// whose prefix unambiguously names one of this project's own known
// non-stable release formats (see .github/workflows/{nightly,prerelease}.yml)
// must stamp that inferred channel via SetChannel instead of silently
// registering onto stable. Concretely, a real nightly-installed daemon
// (version "nightly-<sha>") must never have its own catalog row default to
// tracking "stable" -- a short git SHA carries no numeric-dot version core,
// so this cannot rely on the generic resolvers.ChannelForTag classifier.
func TestEnsureRegistered_ChannelInference_FallsBackToVersionPrefix(t *testing.T) {
	testCases := []struct {
		name          string
		version       string
		wantSetCalled bool
		wantChannel   string
	}{
		{name: "nightly build infers nightly channel", version: "nightly-a1b2c3d", wantSetCalled: true, wantChannel: "nightly"},
		{name: "beta build infers beta channel", version: "beta-25.10", wantSetCalled: true, wantChannel: "beta"},
		{name: "hotfix build infers hotfix channel", version: "hotfix-25.9.1", wantSetCalled: true, wantChannel: "hotfix"},
		{name: "stable build never calls SetChannel", version: "stable-25.9.2", wantSetCalled: false},
		{name: "bare semver with no recognized prefix never calls SetChannel", version: "26.5.0", wantSetCalled: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockArrow{
				ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
				SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
			}
			var setCalled bool
			var gotChannel string
			m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
				setCalled = true
				gotChannel = channel
				return nil
			}

			err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, tc.version, "")

			require.NoError(t, err)
			assert.Equal(t, tc.wantSetCalled, setCalled)
			if tc.wantSetCalled {
				assert.Equal(t, tc.wantChannel, gotChannel)
			}
		})
	}
}

// TestEnsureRegistered_ConfiguredChannel_OverridesVersionInference proves an
// explicitly configured self_update_channel always wins outright over
// whatever the running version's own prefix would otherwise infer.
func TestEnsureRegistered_ConfiguredChannel_OverridesVersionInference(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	var gotChannel string
	m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
		gotChannel = channel
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "nightly-a1b2c3d", "rc")

	require.NoError(t, err)
	assert.Equal(t, "rc", gotChannel)
}

func TestEnsureRegistered_SkipsWhenPresent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.SeedFn = func(context.Context, domain.Namespace, []byte) error {
		t.Fatal("Seed must not be called when the self-arrow already exists")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
}

// TestEnsureRegistered_ChannelConfigured_StampsChannelOnSteadyStateBoot covers
// the steady-state boot: the self row already exists at the version currently
// running, so the early-return path must still stamp a non-empty configured
// channel via SetChannel instead of skipping it entirely -- an operator who
// sets self_update_channel and restarts an already-up-to-date daemon must see
// it take effect immediately, not on the next version change.
func TestEnsureRegistered_ChannelConfigured_StampsChannelOnSteadyStateBoot(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	var capturedChannel string
	m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
		capturedChannel = channel
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.NoError(t, err)
	assert.Equal(t, "rc", capturedChannel)
}

// TestEnsureRegistered_EmptyChannel_ExistsBranch_NeverCallsSetChannel mirrors
// TestEnsureRegistered_EmptyChannel_NeverCallsSetChannel for the steady-state
// (already-registered) boot path.
func TestEnsureRegistered_EmptyChannel_ExistsBranch_NeverCallsSetChannel(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		t.Fatal("SetChannel must not be called when no channel is configured")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
}

// TestEnsureRegistered_UsesEmbeddedManifestNotNetworkResolve guards the whole
// point of this seam: registering quiver.core's own manifest must never
// trigger arrow.Add's network-resolving ResolveForInstall path. Seed is the
// existing lower-level entry point that parses already-in-hand bytes
// locally (manifold.ParseArrow) and sends AddArrow directly, so asserting
// Seed was called with the embedded manifest's exact bytes — and that Add
// was never called at all — proves no network resolve happened.
func TestEnsureRegistered_UsesEmbeddedManifestNotNetworkResolve(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
	}
	var seededFromRaw bool
	var triggeredNetworkResolve bool
	m.SeedFn = func(_ context.Context, _ domain.Namespace, data []byte) error {
		seededFromRaw = bytes.Equal(data, selfmanifest.Raw())
		return nil
	}
	m.AddFn = func(context.Context, domain.Namespace, models.AddOptions) error {
		triggeredNetworkResolve = true
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "26.5.0", "")

	require.NoError(t, err)
	assert.True(t, seededFromRaw, "Seed must be called with selfmanifest.Raw()'s exact bytes")
	assert.False(t, triggeredNetworkResolve, "Add (which resolves over the network) must never be called")
}

func TestEnsureRegistered_DevBuildSkipsRegistration(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			t.Fatal("Exists must not be called for an unstamped dev build")
			return false, nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "dev", "")

	require.NoError(t, err)
}

func TestEnsureRegistered_EmptyVersionSkipsRegistration(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			t.Fatal("Exists must not be called for an empty, unstamped version")
			return false, nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "", "")

	require.NoError(t, err)
}

func TestEnsureRegistered_ExistsFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("catalog unavailable")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			return false, sentinel
		},
		SeedFn: func(context.Context, domain.Namespace, []byte) error {
			t.Fatal("Seed must not be called when Exists fails")
			return nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_SeedFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("seed failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn: func(context.Context, domain.Namespace, []byte) error {
			return sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestEnsureRegistered_EmptyChannel_NeverCallsSetChannel pins the "no
// preference" default: an unset config value must reproduce today's exact
// behaviour on the first-boot path, with SetChannel never invoked at all.
func TestEnsureRegistered_EmptyChannel_NeverCallsSetChannel(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn: func(context.Context, domain.Namespace, []byte) error {
			return nil
		},
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		t.Fatal("SetChannel must not be called when no channel is configured")
		return nil
	}
	m.CheckVersionNowFn = func(context.Context, domain.Namespace) {
		t.Fatal("CheckVersionNow must not be called when no channel was stamped")
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
}

func TestEnsureRegistered_SetChannelFailsAfterSeed_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("set channel failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn: func(context.Context, domain.Namespace, []byte) error {
			return nil
		},
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		return sentinel
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_SetChannelFailsOnSteadyStateBoot_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("set channel failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		return sentinel
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestPromoteRunningBinary_CopiesExecutableToSelfPath(t *testing.T) {
	home := t.TempDir()
	// Simulate "the currently running binary" with a fake executable file —
	// PromoteRunningBinary takes the source path as a parameter specifically
	// so this doesn't need to exec a real binary to test.
	src := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, os.WriteFile(src, []byte("fake binary bytes"), 0o755))

	err := selfarrow.PromoteRunningBinary(src, home)

	require.NoError(t, err)
	dstBin := binaryName()
	data, readErr := os.ReadFile(filepath.Join(home, "self", dstBin))
	require.NoError(t, readErr)
	assert.Equal(t, "fake binary bytes", string(data))
}

func TestPromoteRunningBinary_SetsExecutablePermission(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644)) // deliberately not executable

	err := selfarrow.PromoteRunningBinary(src, home)

	require.NoError(t, err)
	info, statErr := os.Stat(filepath.Join(home, "self", binaryName()))
	require.NoError(t, statErr)
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode()&0o111, "promoted binary must be executable")
	}
}

// TestPromoteRunningBinary_EmptyHomeDir_UsesProcessSelfPath guards against a
// real production regression: cmd/quiver's daemon command never passes
// WithHomeDir (see internal.WithHomeDir's own doc comment: "An empty dir is
// the same as passing no option at all"), so Container.homeDir is empty on
// every real boot. PromoteRunningBinary must treat that the same way every
// other homeDir/homeDirAt pair in this codebase does — falling back to the
// process home — instead of resolving "{{home}}/self" against a literal
// empty string, which is an unwritable path that would make promotion a
// permanent no-op in production.
func TestPromoteRunningBinary_EmptyHomeDir_UsesProcessSelfPath(t *testing.T) {
	quiverHome := t.TempDir()
	t.Setenv("QUIVER_HOME", quiverHome)

	src := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, os.WriteFile(src, []byte("fake binary bytes"), 0o755))

	err := selfarrow.PromoteRunningBinary(src, "")

	require.NoError(t, err)
	data, readErr := os.ReadFile(filepath.Join(quiverHome, "self", binaryName()))
	require.NoError(t, readErr)
	assert.Equal(t, "fake binary bytes", string(data))
}

func TestPromoteRunningBinary_SrcMissing_ReturnsError(t *testing.T) {
	home := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	err := selfarrow.PromoteRunningBinary(missing, home)

	require.Error(t, err)
}

func TestPromoteRunningBinary_SelfDirUnavailable_ReturnsError(t *testing.T) {
	// home itself is a regular file, so "<home>/self" can never be created.
	home := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(home, []byte("x"), 0o644))
	src := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o755))

	err := selfarrow.PromoteRunningBinary(src, home)

	require.Error(t, err)
}

func TestPromoteRunningBinary_DestUnwritable_ReturnsError(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "quiver")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o755))
	// Pre-create the destination as a directory so writing the binary file
	// there fails.
	require.NoError(t, os.MkdirAll(filepath.Join(home, "self", binaryName()), 0o755))

	err := selfarrow.PromoteRunningBinary(src, home)

	require.Error(t, err)
}

// catalogView builds an ArrowView the way the real read model builds one:
// a BARE namespace on the view itself, with every installed ref carried in
// Versions. Getting this shape wrong in a mock proves nothing about
// production: the production store never puts a ref on ArrowView.Namespace.
// See store.toArrowView.
func catalogView(bare domain.Namespace, refs ...string) models.ArrowView {
	versions := make([]models.VersionView, 0, len(refs))
	for _, ref := range refs {
		versions = append(versions, models.VersionView{Namespace: bare.WithRef(ref)})
	}
	return models.ArrowView{Namespace: bare, Versions: versions}
}

// TestEnsureRegistered_UpgradesExistingRowOnAVersionChange covers the
// post-update boot: a self row already exists at a different version, so
// EnsureRegistered must move it onto the new ref via UpgradeVersionSeeded,
// using the embedded manifest bytes, never Seed (which would collide with
// the still-existing old row) and never a network-resolving Add.
func TestEnsureRegistered_UpgradesExistingRowOnAVersionChange(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{
				catalogView(self, "stable-25.9.0"),
				// A different arrow entirely, must be ignored.
				catalogView("github.com/rabbytesoftware/quiver.desktop", "stable-1.0.0"),
			}, nil
		},
	}
	var gotOld, gotNew domain.Namespace
	var gotData []byte
	m.UpgradeVersionSeededFn = func(_ context.Context, oldNs, newNs domain.Namespace, data []byte) error {
		gotOld, gotNew, gotData = oldNs, newNs, data
		return nil
	}
	m.SeedFn = func(context.Context, domain.Namespace, []byte) error {
		t.Fatal("Seed must not be called when an old self row already exists")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
	assert.Equal(t, self.WithRef("stable-25.9.0"), gotOld)
	assert.Equal(t, self.WithRef("stable-25.9.2"), gotNew)
	assert.Equal(t, selfmanifest.Raw(), gotData)
}

// TestEnsureRegistered_ChannelConfigured_StampsChannelOnUpgrade covers the
// post-update boot with a non-empty configured channel: EnsureRegistered
// must stamp it onto the row UpgradeVersionSeeded just moved, the same way
// it does on the first-boot Seed branch.
func TestEnsureRegistered_ChannelConfigured_StampsChannelOnUpgrade(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		UpgradeVersionSeededFn: func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
			return nil
		},
	}
	var capturedChannel string
	m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
		capturedChannel = channel
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.NoError(t, err)
	assert.Equal(t, "rc", capturedChannel)
}

// TestEnsureRegistered_EmptyChannel_UpgradeBranch_NeverCallsSetChannel
// mirrors TestEnsureRegistered_EmptyChannel_NeverCallsSetChannel for the
// UpgradeVersionSeeded branch.
func TestEnsureRegistered_EmptyChannel_UpgradeBranch_NeverCallsSetChannel(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		UpgradeVersionSeededFn: func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
			return nil
		},
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		t.Fatal("SetChannel must not be called when no channel is configured")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.NoError(t, err)
}

// TestEnsureRegistered_IgnoresTheBareGroupingNamespace pins the exact defect
// RetireStale once shipped with, now against currentSelfRow: the catalog
// view's own namespace carries no ref, and treating it as a match would send
// an empty, unusable oldNs into UpgradeVersionSeeded.
func TestEnsureRegistered_IgnoresTheBareGroupingNamespace(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
	}
	var gotOld domain.Namespace
	m.UpgradeVersionSeededFn = func(_ context.Context, oldNs, _ domain.Namespace, _ []byte) error {
		gotOld = oldNs
		return nil
	}

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", ""))

	assert.Equal(t, self.WithRef("stable-25.9.0"), gotOld)
}

// TestEnsureRegistered_NoVersionsIsFirstBoot covers a catalog row that exists
// with no installed refs under it: there is nothing ref-qualified to move
// onto, so this is treated as a genuine first boot and seeded fresh.
func TestEnsureRegistered_NoVersionsIsFirstBoot(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{Namespace: self}}, nil
		},
	}
	var seeded bool
	m.SeedFn = func(context.Context, domain.Namespace, []byte) error {
		seeded = true
		return nil
	}
	m.UpgradeVersionSeededFn = func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
		t.Fatal("UpgradeVersionSeeded must not be called for a view carrying no versions")
		return nil
	}

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", ""))
	assert.True(t, seeded)
}

func TestEnsureRegistered_ListFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("catalog unavailable")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return nil, sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_UpgradeVersionSeededFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("upgrade failed")
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		UpgradeVersionSeededFn: func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
			return sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_SetChannelFailsAfterUpgrade_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("set channel failed")
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		UpgradeVersionSeededFn: func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
			return nil
		},
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		return sentinel
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-25.9.2", "rc")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestEnsureRegistered_SeedPath_MarksRuntimeReady covers the first-ever
// registration: successfully seeding the row is unambiguous proof
// quiver.core is running, so EnsureRegistered must land its runtime
// aggregate at Ready without an install ever having run, passing no
// lastReturn since none exists yet.
func TestEnsureRegistered_SeedPath_MarksRuntimeReady(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	var markedReady domain.Namespace
	var gotLastReturn *domainRuntime.Return
	rt.MarkReadyFn = func(_ context.Context, ns domain.Namespace, lastReturn *domainRuntime.Return) error {
		markedReady = ns
		gotLastReturn = lastReturn
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.NoError(t, err)
	assert.Equal(t, self.WithRef("stable-25.9.2"), markedReady)
	assert.Nil(t, gotLastReturn)
}

// TestEnsureRegistered_UpgradePath_MarksNewRowReady covers the post-update
// boot: the runtime aggregate marked ready must be the NEW row
// UpgradeVersionSeeded just created, never the old one being retired.
func TestEnsureRegistered_UpgradePath_MarksNewRowReady(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		UpgradeVersionSeededFn: func(context.Context, domain.Namespace, domain.Namespace, []byte) error {
			return nil
		},
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	var markedReady domain.Namespace
	rt.MarkReadyFn = func(_ context.Context, ns domain.Namespace, _ *domainRuntime.Return) error {
		markedReady = ns
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.NoError(t, err)
	assert.Equal(t, self.WithRef("stable-25.9.2"), markedReady)
}

// TestEnsureRegistered_ExistsPath_MarksReadyWhenNotAlready covers a
// defensive edge case on the steady-state boot: the row already exists at
// the running version, but its runtime aggregate still reports absent (for
// example, a boot right after this feature first shipped). EnsureRegistered
// must still mark it ready rather than leaving it stuck.
func TestEnsureRegistered_ExistsPath_MarksReadyWhenNotAlready(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	var markReadyCalled bool
	rt.MarkReadyFn = func(context.Context, domain.Namespace, *domainRuntime.Return) error {
		markReadyCalled = true
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.NoError(t, err)
	assert.True(t, markReadyCalled)
}

// TestEnsureRegistered_ExistsPath_AlreadyReady_SkipsMarkReady is the guard
// this whole feature exists for: EnsureRegistered runs on every boot, and
// ready -> ready is not a valid domain.ArrowState transition, so calling
// MarkReady again on an already-ready row must never happen -- it would
// otherwise raise a harmless-but-noisy validation error every boot after
// the first.
func TestEnsureRegistered_ExistsPath_AlreadyReady_SkipsMarkReady(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
	}
	rt.MarkReadyFn = func(context.Context, domain.Namespace, *domainRuntime.Return) error {
		t.Fatal("MarkReady must not be called when the runtime is already ready")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.NoError(t, err)
}

// TestEnsureRegistered_ChannelAndReady_BothApplied confirms finishRegistration
// applies every follow-up on a single successful path, not just the first.
func TestEnsureRegistered_ChannelAndReady_BothApplied(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	var gotChannel string
	m.SetChannelFn = func(_ context.Context, _ domain.Namespace, channel string, _ string) error {
		gotChannel = channel
		return nil
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
	}
	var markReadyCalled bool
	rt.MarkReadyFn = func(context.Context, domain.Namespace, *domainRuntime.Return) error {
		markReadyCalled = true
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "rc")

	require.NoError(t, err)
	assert.Equal(t, "rc", gotChannel)
	assert.True(t, markReadyCalled)
}

// TestEnsureRegistered_SetChannelFails_MarkReadyNeverCalled pins
// finishRegistration's ordering: stampConfiguredChannel runs first, and its
// failure short-circuits before markReadyIfAbsent ever runs.
func TestEnsureRegistered_SetChannelFails_MarkReadyNeverCalled(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	m.SetChannelFn = func(context.Context, domain.Namespace, string, string) error {
		return errors.New("set channel failed")
	}
	rt := &mocks.MockRuntime{}
	rt.MarkReadyFn = func(context.Context, domain.Namespace, *domainRuntime.Return) error {
		t.Fatal("MarkReady must not be called when stampConfiguredChannel fails first")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "rc")

	require.Error(t, err)
}

// TestEnsureRegistered_GetStateFails_ReturnsWrappedError matches this whole
// function's existing contract: every step returns a wrapped error rather
// than swallowing it, relying on the caller (Container.Start) to log it and
// treat self-registration as non-fatal.
func TestEnsureRegistered_GetStateFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("runtime store unavailable")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return "", sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestEnsureRegistered_MarkReadyFails_ReturnsWrappedError covers a MarkReady
// failure that reaches through the guard anyway (for example a state change
// racing with GetState): the error is wrapped and returned like every other
// step in this function, never swallowed -- EnsureRegistered's own error
// contract does not change here, so Container.Start's existing non-fatal
// slog.WarnContext handling still covers it.
func TestEnsureRegistered_MarkReadyFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("mark ready failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		SeedFn:   func(context.Context, domain.Namespace, []byte) error { return nil },
	}
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateAbsent, nil
		},
		MarkReadyFn: func(context.Context, domain.Namespace, *domainRuntime.Return) error {
			return sentinel
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-25.9.2", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// TestPromoteRunningBinary_SelfPathIsNotRewritten covers the routine case
// quiver.desktop creates: every sidecar spawn after the first runs the binary
// AT the self path (SidecarManager::spawn prefers it over its bundled seed),
// so promotion is asked to overwrite the file it is reading. On Linux that
// write is an ETXTBSY; here it is simply not attempted.
func TestPromoteRunningBinary_SelfPathIsNotRewritten(t *testing.T) {
	home := t.TempDir()
	selfDir, err := paths.SelfAt(home)
	require.NoError(t, err)

	running := filepath.Join(selfDir, binaryName())
	require.NoError(t, os.WriteFile(running, []byte("the build that is running"), 0o755))

	require.NoError(t, selfarrow.PromoteRunningBinary(running, home))

	got, err := os.ReadFile(running)
	require.NoError(t, err)
	assert.Equal(t, "the build that is running", string(got))
}

// TestPromoteRunningBinary_CopiesADifferentBinary is the ordinary path: the
// running build lives elsewhere (a vault workdir after a self-update, or
// quiver.desktop's bundled seed on a first launch) and its bytes are copied
// to the stable path a cold start reads from.
func TestPromoteRunningBinary_CopiesADifferentBinary(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(src, []byte("the newly downloaded build"), 0o755))

	require.NoError(t, selfarrow.PromoteRunningBinary(src, home))

	selfDir, err := paths.SelfAt(home)
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(selfDir, binaryName()))
	require.NoError(t, err)
	assert.Equal(t, "the newly downloaded build", string(got))
}
