package selfarrow_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	engineMocks "github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// binaryName mirrors selfarrow's own unexported helper so the tests can
// predict the promoted filename without depending on selfarrow's internals.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "quiver.exe"
	}
	return "quiver"
}

func selfNs() domain.Namespace {
	self, _ := metadata.GetSelfNamespaces()
	return self
}

type adoptCall struct {
	ns       domain.Namespace
	kind     domain.SelectorKind
	resolved domain.Resolved
	manifest []byte
	filename string
}

// recordingCatalog is a MockArrow that records every Adopt, Remove and
// CheckVersionNow it receives.
func recordingCatalog(views ...models.ArrowView) (*mocks.MockArrow, *[]adoptCall, *[]domain.Namespace, *[]domain.Namespace) {
	var adopted []adoptCall
	var removed []domain.Namespace
	var checked []domain.Namespace
	m := &mocks.MockArrow{
		AdoptFn: func(_ context.Context, ns domain.Namespace, kind domain.SelectorKind, resolved domain.Resolved, manifest []byte, filename string) error {
			adopted = append(adopted, adoptCall{ns: ns, kind: kind, resolved: resolved, manifest: manifest, filename: filename})
			return nil
		},
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) { return views, nil },
		RemoveFn: func(_ context.Context, ns domain.Namespace) error {
			removed = append(removed, ns)
			return nil
		},
		CheckVersionNowFn: func(_ context.Context, ns domain.Namespace) { checked = append(checked, ns) },
	}
	return m, &adopted, &removed, &checked
}

// catalogView builds an ArrowView the way the read model does: a bare
// namespace on the view, every catalogued ref in Versions.
func catalogView(bare domain.Namespace, refs ...string) models.ArrowView {
	versions := make([]models.VersionView, 0, len(refs))
	for _, ref := range refs {
		versions = append(versions, models.VersionView{Namespace: bare.WithRef(ref)})
	}
	return models.ArrowView{Namespace: bare, Versions: versions}
}

func TestEnsureRegistered_UnstampedBuild_IsANoOp(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		channel string
	}{
		{name: "empty version and channel", version: "", channel: ""},
		{name: "empty version with a channel", version: "", channel: "nightly-latest"},
		{name: "dev version", version: "dev", channel: ""},
		{name: "dev version with a channel", version: "dev", channel: "beta"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.MockArrow{
				AdoptFn: func(context.Context, domain.Namespace, domain.SelectorKind, domain.Resolved, []byte, string) error {
					t.Fatal("an unstamped build must not register")
					return nil
				},
				ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
					t.Fatal("an unstamped build must not touch the catalog")
					return nil, nil
				},
			}
			rt := &mocks.MockRuntime{
				MarkReadyFn: func(context.Context, domain.Namespace) error {
					t.Fatal("an unstamped build must not mark anything ready")
					return nil
				},
			}

			require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, rt, tc.version, "abc", tc.channel))
		})
	}
}

func TestEnsureRegistered_Channel_AdoptsTheChannelIdentity(t *testing.T) {
	m, adopted, _, checked := recordingCatalog()
	var markedReady domain.Namespace
	rt := &mocks.MockRuntime{
		MarkReadyFn: func(_ context.Context, ns domain.Namespace) error {
			markedReady = ns
			return nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, rt, "nightly-latest", "c0ffee", "nightly-latest")

	require.NoError(t, err)
	want := selfNs().WithRef("nightly-latest")
	require.Len(t, *adopted, 1)
	got := (*adopted)[0]
	assert.Equal(t, want, got.ns)
	assert.Equal(t, domain.SelectorPointerChannel, got.kind, "a build published under its channel's name follows that rolling tag")
	assert.Equal(t, domain.Resolved{Ref: "nightly-latest", Commit: "c0ffee", Fingerprint: "c0ffee"}, got.resolved)
	assert.Equal(t, selfmanifest.Raw(), got.manifest)
	assert.Equal(t, "ARROW.md", got.filename)
	assert.Equal(t, want, markedReady)
	assert.Equal(t, []domain.Namespace{want}, *checked)
}

// A build that declares no channel is registered as a pin of its own ref.
func TestEnsureRegistered_NoChannel_AdoptsAPinOfTheVersion(t *testing.T) {
	m, adopted, _, _ := recordingCatalog()

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-26.5.1", "abc", "")

	require.NoError(t, err)
	require.Len(t, *adopted, 1)
	assert.Equal(t, selfNs().WithRef("stable-26.5.1"), (*adopted)[0].ns)
	assert.Equal(t, domain.SelectorTagPin, (*adopted)[0].kind)
	assert.Equal(t, "stable-26.5.1", (*adopted)[0].resolved.Ref)
}

// The boot after an update keeps the identity and adopts the new build's
// state on it.
func TestEnsureRegistered_NewCommitOnTheSameIdentity_AdoptsTheNewState(t *testing.T) {
	m, adopted, removed, _ := recordingCatalog(catalogView(selfNs(), "beta"))

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "beta-26.5-4", "aaa", "beta"))
	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "beta-26.5-5", "bbb", "beta"))

	require.Len(t, *adopted, 2)
	assert.Equal(t, (*adopted)[0].ns, (*adopted)[1].ns)
	assert.Equal(t, domain.Resolved{Ref: "beta-26.5-5", Commit: "bbb", Fingerprint: "bbb"}, (*adopted)[1].resolved)
	assert.Empty(t, *removed)
}

func TestChannel_ConfiguredOverridesTheBuild(t *testing.T) {
	testCases := []struct {
		name       string
		configured string
		built      string
		want       string
	}{
		{name: "configured wins", configured: "beta", built: "stable", want: "beta"},
		{name: "build when nothing is configured", configured: "", built: "stable", want: "stable"},
		{name: "neither", configured: "", built: "", want: ""},
		{name: "configured without a build channel", configured: "rc", built: "", want: "rc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, selfarrow.Channel(tc.configured, tc.built))
		})
	}
}

func TestEnsureRegistered_ConfiguredChannel_IsTheIdentity(t *testing.T) {
	m, adopted, _, _ := recordingCatalog()

	channel := selfarrow.Channel("rc", "stable")
	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-26.5.1", "abc", channel))

	require.Len(t, *adopted, 1)
	assert.Equal(t, selfNs().WithRef("rc"), (*adopted)[0].ns)
	assert.Equal(t, domain.SelectorOrderedChannel, (*adopted)[0].kind, "a release tag published under another name is a member of an ordered channel")
}

// Earlier builds filed the core under other identities; after adopting the
// current one, every other self row goes, and nothing else does.
func TestEnsureRegistered_RemovesOtherSelfRows(t *testing.T) {
	other := domain.Namespace("github.com/someone/else")
	m, _, removed, _ := recordingCatalog(
		catalogView(selfNs(), "nightly-abc123", "stable-26.5.1", "nightly-latest"),
		catalogView(other, "v1"),
	)

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "nightly-latest", "abc", "nightly-latest")

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{
		selfNs().WithRef("nightly-abc123"),
		selfNs().WithRef("stable-26.5.1"),
	}, *removed)
}

func TestEnsureRegistered_OnlyTheCurrentIdentity_RemovesNothing(t *testing.T) {
	m, _, removed, _ := recordingCatalog(catalogView(selfNs(), "stable"))

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-26.5.1", "abc", "stable"))

	assert.Empty(t, *removed)
}

// A row already gone by the time it is removed is what removal wanted.
func TestEnsureRegistered_StaleRowAlreadyGone_IsNotAnError(t *testing.T) {
	m, _, _, _ := recordingCatalog(catalogView(selfNs(), "stable-26.5.1", "stable"))
	m.RemoveFn = func(context.Context, domain.Namespace) error {
		return fmt.Errorf("remove: %w", apperrors.ErrNotFound)
	}

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-26.5.1", "abc", "stable"))
}

func TestEnsureRegistered_Failures(t *testing.T) {
	sentinel := errors.New("boom")
	testCases := []struct {
		name          string
		catalog       func(m *mocks.MockArrow)
		runtime       *mocks.MockRuntime
		wantMarkReady bool
	}{
		{
			name: "adopt fails",
			catalog: func(m *mocks.MockArrow) {
				m.AdoptFn = func(context.Context, domain.Namespace, domain.SelectorKind, domain.Resolved, []byte, string) error {
					return sentinel
				}
			},
			runtime: &mocks.MockRuntime{},
		},
		{
			name:    "reading the runtime state fails",
			catalog: func(*mocks.MockArrow) {},
			runtime: &mocks.MockRuntime{
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) { return "", sentinel },
			},
		},
		{
			name:    "marking ready fails",
			catalog: func(*mocks.MockArrow) {},
			runtime: &mocks.MockRuntime{
				MarkReadyFn: func(context.Context, domain.Namespace) error { return sentinel },
			},
			wantMarkReady: true,
		},
		{
			name: "listing the catalog fails",
			catalog: func(m *mocks.MockArrow) {
				m.ListFn = func(context.Context, *bool) ([]models.ArrowView, error) { return nil, sentinel }
			},
			runtime:       &mocks.MockRuntime{},
			wantMarkReady: true,
		},
		{
			name: "removing a stale row fails",
			catalog: func(m *mocks.MockArrow) {
				m.ListFn = func(context.Context, *bool) ([]models.ArrowView, error) {
					return []models.ArrowView{catalogView(selfNs(), "stable-26.5.1")}, nil
				}
				m.RemoveFn = func(context.Context, domain.Namespace) error { return sentinel }
			},
			runtime:       &mocks.MockRuntime{},
			wantMarkReady: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, _ := recordingCatalog()
			tc.catalog(m)
			markReady := tc.runtime.MarkReadyFn
			var markedReady bool
			tc.runtime.MarkReadyFn = func(ctx context.Context, ns domain.Namespace) error {
				markedReady = true
				if markReady != nil {
					return markReady(ctx, ns)
				}
				return nil
			}

			err := selfarrow.EnsureRegistered(context.Background(), m, tc.runtime, "stable-26.5.1", "abc", "stable")

			require.ErrorIs(t, err, sentinel)
			assert.Equal(t, tc.wantMarkReady, markedReady)
		})
	}
}

func TestEnsureRegistered_ReadyRuntime_SkipsMarkReady(t *testing.T) {
	m, _, _, _ := recordingCatalog()
	rt := &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
			return domain.ArrowStateReady, nil
		},
		MarkReadyFn: func(context.Context, domain.Namespace) error {
			t.Fatal("ready -> ready is not a valid transition")
			return nil
		},
	}

	require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-26.5.1", "abc", "stable"))
}

// settledRuntime records what EnsureRegistered did to the adopted row's
// runtime, starting from state.
type settledRuntime struct {
	*mocks.MockRuntime
	markedReady []domain.Namespace
	cleared     []domain.Namespace
}

func runtimeIn(state domain.ArrowState) *settledRuntime {
	r := &settledRuntime{}
	r.MockRuntime = &mocks.MockRuntime{
		GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) { return state, nil },
		MarkReadyFn: func(_ context.Context, ns domain.Namespace) error {
			r.markedReady = append(r.markedReady, ns)
			return nil
		},
		ClearVersionBadgeFn: func(_ context.Context, ns domain.Namespace) error {
			r.cleared = append(r.cleared, ns)
			return nil
		},
	}
	return r
}

// withAvailable lists ns carrying available, the way the read model does
// once the adopted row's version check has found something ahead of it.
func withAvailable(ns domain.Namespace, available *domain.Available) models.ArrowView {
	return models.ArrowView{
		Namespace: ns.BareNamespace(),
		Versions: []models.VersionView{{
			Namespace: ns,
			Metadata:  domain.Arrow{Namespace: ns, Available: available},
		}},
	}
}

// After a self-update the relaunched build adopts its new state on the same
// row; the Outdated badge the drift check set before the update goes with
// it, offline, so the row settles at Ready without waiting for a check.
func TestEnsureRegistered_SettlesTheRuntimeAtReady(t *testing.T) {
	ns := selfNs().WithRef("stable")
	testCases := []struct {
		name        string
		state       domain.ArrowState
		views       []models.ArrowView
		wantReady   bool
		wantCleared bool
	}{
		{name: "absent is marked ready", state: domain.ArrowStateAbsent, wantReady: true},
		{name: "ready is untouched", state: domain.ArrowStateReady},
		{name: "outdated with nothing available is cleared", state: domain.ArrowStateOutdated, views: []models.ArrowView{withAvailable(ns, nil)}, wantCleared: true},
		{name: "outdated missing from the read model is cleared", state: domain.ArrowStateOutdated, wantCleared: true},
		{
			name:  "outdated with a newer release still available keeps its badge",
			state: domain.ArrowStateOutdated,
			views: []models.ArrowView{withAvailable(ns, &domain.Available{Ref: "stable-26.5.3", Commit: "ccc"})},
		},
		{name: "running is untouched", state: domain.ArrowStateRunning},
		{name: "updating is untouched", state: domain.ArrowStateUpdating},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, _ := recordingCatalog(tc.views...)
			rt := runtimeIn(tc.state)

			require.NoError(t, selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-26.5.2", "bbb", "stable"))

			var wantReady, wantCleared []domain.Namespace
			if tc.wantReady {
				wantReady = []domain.Namespace{ns}
			}
			if tc.wantCleared {
				wantCleared = []domain.Namespace{ns}
			}
			assert.Equal(t, wantReady, rt.markedReady)
			assert.Equal(t, wantCleared, rt.cleared)
		})
	}
}

func TestEnsureRegistered_SettlingAnOutdatedRowFails(t *testing.T) {
	sentinel := errors.New("boom")
	testCases := []struct {
		name    string
		catalog func(m *mocks.MockArrow)
		runtime func(r *settledRuntime)
	}{
		{
			name: "reading what is available fails",
			catalog: func(m *mocks.MockArrow) {
				m.ListFn = func(context.Context, *bool) ([]models.ArrowView, error) { return nil, sentinel }
			},
			runtime: func(*settledRuntime) {},
		},
		{
			name:    "clearing the badge fails",
			catalog: func(*mocks.MockArrow) {},
			runtime: func(r *settledRuntime) {
				r.ClearVersionBadgeFn = func(context.Context, domain.Namespace) error { return sentinel }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, _ := recordingCatalog()
			tc.catalog(m)
			rt := runtimeIn(domain.ArrowStateOutdated)
			tc.runtime(rt)

			err := selfarrow.EnsureRegistered(context.Background(), m, rt, "stable-26.5.2", "bbb", "stable")

			require.ErrorIs(t, err, sentinel)
		})
	}
}

// One stale row that cannot be removed does not keep the others around.
func TestEnsureRegistered_RemoveFails_StillRemovesTheRest(t *testing.T) {
	sentinel := errors.New("busy")
	m, _, _, _ := recordingCatalog(catalogView(selfNs(), "nightly-abc123", "stable-26.5.1", "stable"))
	var attempted []domain.Namespace
	m.RemoveFn = func(_ context.Context, ns domain.Namespace) error {
		attempted = append(attempted, ns)
		if len(attempted) == 1 {
			return sentinel
		}
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, &mocks.MockRuntime{}, "stable-26.5.2", "bbb", "stable")

	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, []domain.Namespace{
		selfNs().WithRef("nightly-abc123"),
		selfNs().WithRef("stable-26.5.1"),
	}, attempted)
}

// ─── Regression: the core stays in the user's library across updates ────────

// libraryCatalog is the real arrow repository over an in-memory read model,
// with the detached version check recorded instead of run.
type libraryCatalog struct {
	arrowRepo.Arrow
}

func (libraryCatalog) CheckVersionNow(context.Context, domain.Namespace) {}

func newLibrary(t *testing.T) (libraryCatalog, asynx.Asynx[domain.Arrow]) {
	t.Helper()
	return newLibraryWith(t, &engineMocks.Manifold{ParseArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Quiver Core"}}})
}

func newLibraryWith(t *testing.T, m *engineMocks.Manifold) (libraryCatalog, asynx.Asynx[domain.Arrow]) {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	cat, err := arrowRepo.New(db, ax, &engineMocks.Vault{}, m, nil)
	require.NoError(t, err)
	return libraryCatalog{Arrow: cat}, ax
}

// library is what the desktop lists: GET /v0/arrow?user_installed=true.
func library(t *testing.T, cat libraryCatalog) []domain.Namespace {
	t.Helper()
	userInstalled := true
	views, err := cat.List(context.Background(), &userInstalled)
	require.NoError(t, err)
	var out []domain.Namespace
	for _, view := range views {
		for _, version := range view.Versions {
			out = append(out, version.Namespace)
		}
	}
	return out
}

type boot struct {
	version string
	commit  string
	channel string
}

func TestEnsureRegistered_CoreStaysInTheLibraryAcrossUpdates(t *testing.T) {
	testCases := []struct {
		name  string
		first boot
		next  boot
		want  domain.Namespace
	}{
		{
			name:  "same identity, new version and commit",
			first: boot{version: "beta-26.5-4", commit: "aaa", channel: "beta"},
			next:  boot{version: "beta-26.5-5", commit: "bbb", channel: "beta"},
			want:  selfNs().WithRef("beta"),
		},
		{
			name:  "rolling tag, new commit",
			first: boot{version: "nightly-latest", commit: "aaa", channel: "nightly-latest"},
			next:  boot{version: "nightly-latest", commit: "bbb", channel: "nightly-latest"},
			want:  selfNs().WithRef("nightly-latest"),
		},
		{
			name:  "different identity",
			first: boot{version: "stable-26.5.1", commit: "aaa", channel: ""},
			next:  boot{version: "stable-26.5.2", commit: "bbb", channel: "stable"},
			want:  selfNs().WithRef("stable"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cat, ax := newLibrary(t)
			ctx := context.Background()

			require.NoError(t, selfarrow.EnsureRegistered(ctx, cat, &mocks.MockRuntime{}, tc.first.version, tc.first.commit, tc.first.channel))
			require.NotEmpty(t, library(t, cat))
			require.NoError(t, selfarrow.EnsureRegistered(ctx, cat, &mocks.MockRuntime{}, tc.next.version, tc.next.commit, tc.next.channel))

			assert.Equal(t, []domain.Namespace{tc.want}, library(t, cat))
			row, err := ax.Get(ctx, tc.want.String())
			require.NoError(t, err)
			assert.True(t, row.UserInstalled)
			assert.Equal(t, domain.Resolved{Ref: tc.next.version, Commit: tc.next.commit, Fingerprint: tc.next.commit}, row.Resolved)
		})
	}
}

// A self row left without the user-installed flag, as the bug left it, is
// back in the library after the next boot.
func TestEnsureRegistered_RowLeftOutOfTheLibrary_IsRepaired(t *testing.T) {
	ns := selfNs().WithRef("stable")
	quiverCore := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Quiver Core"}}
	cat, ax := newLibraryWith(t, &engineMocks.Manifold{
		ParseArrowResult:           quiverCore,
		SnapshotResult:             domain.RefSnapshot{Branches: map[string]string{"stable": "aaa"}},
		ResolveArrowAtCommitResult: quiverCore,
	})
	ctx := context.Background()
	_, err := cat.AddDependency(ctx, ns)
	require.NoError(t, err)
	require.Empty(t, library(t, cat))

	require.NoError(t, selfarrow.EnsureRegistered(ctx, cat, &mocks.MockRuntime{}, "stable-26.5.2", "bbb", "stable"))

	assert.Equal(t, []domain.Namespace{ns}, library(t, cat))
	row, err := ax.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.True(t, row.UserInstalled)
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

func TestBinaryPath_EmptyHomeDir_UsesProcessSelfPath(t *testing.T) {
	t.Setenv("QUIVER_HOME", t.TempDir())

	got, err := selfarrow.BinaryPath("")

	require.NoError(t, err)
	assert.Equal(t, "self", filepath.Base(filepath.Dir(got)))
}
