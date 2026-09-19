package selfarrow_test

import (
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
	var addedNS domain.Namespace
	m.AddFn = func(_ context.Context, ns domain.Namespace) error {
		addedNS = ns
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/rabbytesoftware/quiver.core@stable-25.9.2"), addedNS)
}

func TestEnsureRegistered_SkipsWhenPresent(t *testing.T) {
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return true, nil },
	}
	m.AddFn = func(context.Context, domain.Namespace) error {
		t.Fatal("Add must not be called when the self-arrow already exists")
		return nil
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.NoError(t, err)
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
		AddFn: func(context.Context, domain.Namespace) error {
			t.Fatal("Add must not be called when Exists fails")
			return nil
		},
	}

	err := selfarrow.EnsureRegistered(context.Background(), m, "stable-25.9.2")

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestEnsureRegistered_AddFails_ReturnsWrappedError(t *testing.T) {
	sentinel := errors.New("add failed")
	m := &mocks.MockArrow{
		ExistsFn: func(context.Context, domain.Namespace) (bool, error) { return false, nil },
		AddFn: func(context.Context, domain.Namespace) error {
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

func TestRetireStale_RemovesOtherSelfRefsKeepsCurrent(t *testing.T) {
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{
				{Namespace: selfarrow.Namespace.WithRef("stable-25.9.0")},
				{Namespace: selfarrow.Namespace.WithRef("stable-25.9.2")},
				{Namespace: "github.com/rabbytesoftware/quiver.desktop@stable-1.0.0"}, // different arrow entirely — must survive
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
	assert.Equal(t, []domain.Namespace{selfarrow.Namespace.WithRef("stable-25.9.0")}, removed)
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
	m := &mocks.MockArrow{
		ListFn: func(context.Context, *bool) ([]models.ArrowView, error) {
			return []models.ArrowView{
				{Namespace: selfarrow.Namespace.WithRef("stable-25.9.0")},
			}, nil
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
