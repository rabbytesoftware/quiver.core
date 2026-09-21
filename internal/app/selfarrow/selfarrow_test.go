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
	"github.com/rabbytesoftware/quiver.core/internal/core/selfmanifest"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
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

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/rabbytesoftware/quiver.core@stable-25.9.2"), seededNS)
}

func TestEnsureRegistered_SkipsWhenPresent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.SeedFn = func(context.Context, domain.Namespace, []byte) error {
		t.Fatal("Seed must not be called when the self-arrow already exists")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

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
	m.AddFn = func(context.Context, domain.Namespace) error {
		triggeredNetworkResolve = true
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "26.5.0")

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

	err := selfarrow.EnsureRegistered(context.Background(), m, "dev")

	require.NoError(t, err)
}

func TestEnsureRegistered_EmptyVersionSkipsRegistration(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) {
			t.Fatal("Exists must not be called for an empty, unstamped version")
			return false, nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "")

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

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

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

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

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
// Versions. Getting this shape wrong in a mock is what let RetireStale ship
// as a permanent no-op — the production store never puts a ref on
// ArrowView.Namespace, so a test that does proves nothing about production.
// See store.toArrowView.
func catalogView(bare domain.Namespace, refs ...string) models.ArrowView {
	versions := make([]models.VersionView, 0, len(refs))
	for _, ref := range refs {
		versions = append(versions, models.VersionView{Namespace: bare.WithRef(ref)})
	}
	return models.ArrowView{Namespace: bare, Versions: versions}
}

func TestRetireStale_RemovesOtherSelfRefsKeepsCurrent(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{
				catalogView(self, "stable-25.9.0", "stable-25.9.2"),
				// A different arrow entirely — must survive, refs and all.
				catalogView("github.com/rabbytesoftware/quiver.desktop", "stable-1.0.0"),
			}, nil
		},
	}
	var removed []domain.Namespace
	m.RemoveFn = func(_ context.Context, ns domain.Namespace) error {
		removed = append(removed, ns)
		return nil
	}

	err := selfarrow.RetireStale(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{self.WithRef("stable-25.9.0")}, removed)
}

// TestRetireStale_IgnoresTheBareGroupingNamespace pins the exact defect this
// function shipped with: the catalog view's own namespace carries no ref, and
// treating it as one retired nothing at all, however many stale versions sat
// underneath it.
func TestRetireStale_IgnoresTheBareGroupingNamespace(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
	}
	var removed []domain.Namespace
	m.RemoveFn = func(_ context.Context, ns domain.Namespace) error {
		removed = append(removed, ns)
		return nil
	}

	require.NoError(t, selfarrow.RetireStale(context.Background(), m, "stable-25.9.2"))

	assert.Equal(t, []domain.Namespace{self.WithRef("stable-25.9.0")}, removed,
		"the stale ref must be retired, and the bare grouping namespace must never be")
}

// TestRetireStale_NoVersionsIsNoOp covers a catalog row that exists with no
// installed refs under it: there is nothing ref-qualified to retire, and the
// bare namespace must not be mistaken for one.
func TestRetireStale_NoVersionsIsNoOp(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{{Namespace: self}}, nil
		},
		RemoveFn: func(context.Context, domain.Namespace) error {
			t.Fatal("Remove must not be called for a view carrying no versions")
			return nil
		},
	}

	require.NoError(t, selfarrow.RetireStale(context.Background(), m, "stable-25.9.2"))
}

func TestRetireStale_ListFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("catalog unavailable")
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return nil, sentinel
		},
	}

	err := selfarrow.RetireStale(context.Background(), m, "stable-25.9.2")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestRetireStale_RemoveFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("remove failed")
	self, _ := metadata.GetSelfNamespaces()
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{catalogView(self, "stable-25.9.0")}, nil
		},
		RemoveFn: func(context.Context, domain.Namespace) error {
			return sentinel
		},
	}

	err := selfarrow.RetireStale(context.Background(), m, "stable-25.9.2")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestRetireStale_EmptyOrDevVersion_NoOp(t *testing.T) {
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			t.Fatal("List must not be called for an unstamped dev build")
			return nil, nil
		},
	}

	require.NoError(t, selfarrow.RetireStale(context.Background(), m, "dev"))
	require.NoError(t, selfarrow.RetireStale(context.Background(), m, ""))
}
