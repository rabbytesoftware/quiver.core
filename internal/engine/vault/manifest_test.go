package vault

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(
	t *testing.T,
) *store {
	return &store{
		basePath:  t.TempDir(),
		ttl:       time.Hour,
		osVersion: "darwin/arm64",
		locks:     make(map[string]*sync.Mutex),
	}
}

func writeStaleArrowEntry(
	t *testing.T,
	dir string,
	manifest *domain.ArrowManifest,
	indirectDeps []domain.Namespace,
) {
	entry := struct {
		Manifest             *domain.ArrowManifest `json:"manifest"`
		CachedAt             time.Time             `json:"cached_at"`
		OS                   string                `json:"os"`
		IndirectDependencies []domain.Namespace    `json:"indirect_dependencies,omitempty"`
	}{
		Manifest:             manifest,
		CachedAt:             time.Now().Add(-2 * time.Hour),
		OS:                   "darwin/arm64",
		IndirectDependencies: indirectDeps,
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, arrowFilename), data, 0644))
}

func writeStaleQuiverEntry(
	t *testing.T,
	dir string,
	manifest *domain.QuiverManifest,
) {
	entry := struct {
		Manifest *domain.QuiverManifest `json:"manifest"`
		CachedAt time.Time              `json:"cached_at"`
		OS       string                 `json:"os"`
	}{
		Manifest: manifest,
		CachedAt: time.Now().Add(-2 * time.Hour),
		OS:       "darwin/arm64",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, quiverFilename), data, 0644))
}

// getArrow tests

func TestHelperGetArrow_NotCached(t *testing.T) {
	s := newTestStore(t)

	_, _, err := getArrow(s, mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetArrow_Fresh(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test-arrow"}

	_, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)

	got, path, err := getArrow(s, ns)

	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
	assert.Nil(t, got.IndirectDependencies)
}

func TestHelperGetArrow_FreshWithIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test-arrow"}
	indirectDeps := []domain.Namespace{
		domain.Namespace("github.com/foo/bar"),
		domain.Namespace("github.com/baz/qux"),
	}

	_, err := putArrow(s, ns, manifest, indirectDeps)
	require.NoError(t, err)

	got, path, err := getArrow(s, ns)

	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.Equal(t, indirectDeps, got.IndirectDependencies)
	assert.NotEmpty(t, path)
}

func TestHelperGetArrow_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "stale-arrow"}
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	writeStaleArrowEntry(t, nsDir, manifest, nil)

	got, path, err := getArrow(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetArrow_CorruptedJSON(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, arrowFilename), []byte("not-json"), 0644))

	_, _, err := getArrow(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

func TestHelperGetArrow_ReadError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, arrowFilename), 0700))

	_, _, err := getArrow(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
}

// getQuiver tests

func TestHelperGetQuiver_NotCached(t *testing.T) {
	s := newTestStore(t)

	_, _, err := getQuiver(s, mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetQuiver_Fresh(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "test-quiver"}

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	got, path, err := getQuiver(s, ns)

	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetQuiver_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "stale-quiver"}
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	writeStaleQuiverEntry(t, nsDir, manifest)

	got, path, err := getQuiver(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetQuiver_CorruptedJSON(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("not-json"), 0644))

	_, _, err := getQuiver(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

// putArrow tests

func TestHelperPutArrow_CreatesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test-arrow"}

	path, err := putArrow(s, ns, manifest, nil)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestHelperPutArrow_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{Name: "first"}, nil)
	require.NoError(t, err)

	_, err = putArrow(s, ns, &domain.ArrowManifest{Name: "second"}, nil)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestHelperPutArrow_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	s.osVersion = "linux/amd64"
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "meta-arrow"}

	path, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		Manifest *domain.ArrowManifest `json:"manifest"`
		CachedAt time.Time             `json:"cached_at"`
		OS       string                `json:"os"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.Equal(t, "linux/amd64", entry.OS)
	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Name, entry.Manifest.Name)
}

func TestHelperPutArrow_PersistsIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "arrow-with-deps"}
	indirectDeps := []domain.Namespace{
		domain.Namespace("github.com/foo/bar"),
	}

	path, err := putArrow(s, ns, manifest, indirectDeps)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		IndirectDependencies []domain.Namespace `json:"indirect_dependencies,omitempty"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.Equal(t, indirectDeps, entry.IndirectDependencies)
}

func TestHelperPutArrow_MarshalError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{
		Lifecycle: domain.Lifecycle{
			Install: []step.Step{&mocks.BadStep{Unmarshalable: make(chan struct{})}},
		},
	}

	_, err := putArrow(s, ns, manifest, nil)

	assert.Error(t, err)
}

func TestHelperPutArrow_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.WriteFile(filepath.Join(s.basePath, "namespaces"), []byte("block"), 0644))

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)

	assert.Error(t, err)
}

func TestHelperPutArrow_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)

	assert.Error(t, err)
}

func TestHelperPutArrow_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, arrowFilename), 0700))

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)

	assert.Error(t, err)
}

// putQuiver tests

func TestHelperPutQuiver_CreatesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "test-quiver"}

	path, err := putQuiver(s, ns, manifest)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestHelperPutQuiver_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putQuiver(s, ns, &domain.QuiverManifest{Name: "first"})
	require.NoError(t, err)

	_, err = putQuiver(s, ns, &domain.QuiverManifest{Name: "second"})
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestHelperPutQuiver_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	s.osVersion = "windows/amd64"
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "meta-quiver"}

	path, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		Manifest *domain.QuiverManifest `json:"manifest"`
		CachedAt time.Time              `json:"cached_at"`
		OS       string                 `json:"os"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.Equal(t, "windows/amd64", entry.OS)
	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Name, entry.Manifest.Name)
}

func TestHelperPutQuiver_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.WriteFile(filepath.Join(s.basePath, "namespaces"), []byte("block"), 0644))

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

// deleteArrow tests

func TestHelperDeleteArrow_RemovesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{Name: "to-delete"}, nil)
	require.NoError(t, err)

	err = deleteArrow(s, ns)
	require.NoError(t, err)

	_, _, err = getArrow(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperDeleteArrow_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := deleteArrow(s, ns)

	assert.NoError(t, err)
}

func TestHelperDeleteArrow_RemovesDirectoryWhenEmpty(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)
	require.NoError(t, err)

	_, dir, err := s.acquireNamespace(ns)
	require.NoError(t, err)

	err = deleteArrow(s, ns)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestHelperDeleteArrow_PreservesDirectoryWhenQuiverExists(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)
	require.NoError(t, err)
	_, err = putQuiver(s, ns, &domain.QuiverManifest{})
	require.NoError(t, err)

	_, dir, err := s.acquireNamespace(ns)
	require.NoError(t, err)

	err = deleteArrow(s, ns)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.NoError(t, err)
}

// deleteQuiver tests

func TestHelperDeleteQuiver_RemovesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putQuiver(s, ns, &domain.QuiverManifest{Name: "to-delete"})
	require.NoError(t, err)

	err = deleteQuiver(s, ns)
	require.NoError(t, err)

	_, _, err = getQuiver(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperDeleteQuiver_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := deleteQuiver(s, ns)

	assert.NoError(t, err)
}

func TestHelperDeleteQuiver_RemovesDirectoryWhenEmpty(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})
	require.NoError(t, err)

	_, dir, err := s.acquireNamespace(ns)
	require.NoError(t, err)

	err = deleteQuiver(s, ns)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestHelperDeleteQuiver_PreservesDirectoryWhenArrowExists(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{}, nil)
	require.NoError(t, err)
	_, err = putQuiver(s, ns, &domain.QuiverManifest{})
	require.NoError(t, err)

	_, dir, err := s.acquireNamespace(ns)
	require.NoError(t, err)

	err = deleteQuiver(s, ns)
	require.NoError(t, err)

	_, err = os.Stat(dir)
	assert.NoError(t, err)
}

// Namespace path safety

func TestHelperNamespacePath_RejectsTraversal(t *testing.T) {
	s := newTestStore(t)

	_, _, err := s.acquireNamespace(domain.Namespace("../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
