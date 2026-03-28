package vault

import (
	"encoding/json"
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

func writeStaleEntry[T any](
	t *testing.T,
	dir string,
	filename string,
	manifest T,
) {
	entry := vaultEntry[T]{
		CachedAt: time.Now().Add(-2 * time.Hour),
		OS:       "darwin/arm64",
		Manifest: manifest,
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), data, 0644))
}

// getManifest tests

func TestGetManifest_NotCached(t *testing.T) {
	s := newTestStore(t)

	_, _, err := getManifest[domain.ArrowManifest](s, mocks.Namespace(), "arrow.json")

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetManifest_Fresh(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := domain.ArrowManifest{Name: "test-arrow"}

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	require.NoError(t, err)

	got, path, err := getManifest[domain.ArrowManifest](s, ns, "arrow.json")

	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Name)
	assert.NotEmpty(t, path)
}

func TestGetManifest_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := domain.ArrowManifest{Name: "stale-arrow"}
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	writeStaleEntry(t, nsDir, "arrow.json", manifest)

	got, path, err := getManifest[domain.ArrowManifest](s, ns, "arrow.json")

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
	assert.Equal(t, manifest.Name, got.Name)
	assert.NotEmpty(t, path)
}

func TestGetManifest_CorruptedJSON(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, "arrow.json"), []byte("not-json"), 0644))

	_, _, err := getManifest[domain.ArrowManifest](s, ns, "arrow.json")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

func TestGetManifest_ReadError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, "arrow.json"), 0700))

	_, _, err := getManifest[domain.ArrowManifest](s, ns, "arrow.json")

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
}

func TestNamespacePath_RejectsTraversal(t *testing.T) {
	s := newTestStore(t)

	_, _, err := s.acquireNamespace(domain.Namespace("../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// putManifest tests

func TestPutManifest_CreatesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := domain.ArrowManifest{Name: "test-arrow"}

	path, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestPutManifest_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{Name: "first"})
	require.NoError(t, err)

	_, err = putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{Name: "second"})
	require.NoError(t, err)

	got, _, err := getManifest[domain.ArrowManifest](s, ns, "arrow.json")
	require.NoError(t, err)
	assert.Equal(t, "second", got.Name)
}

func TestPutManifest_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	s.osVersion = "linux/amd64"
	ns := mocks.Namespace()
	manifest := domain.ArrowManifest{Name: "meta-arrow"}

	path, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry vaultEntry[domain.ArrowManifest]
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.Equal(t, "linux/amd64", entry.OS)
	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Name, entry.Manifest.Name)
}

func TestPutManifest_MarshalError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := domain.ArrowManifest{
		Lifecycle: domain.Lifecycle{
			Install: []step.Step{&mocks.BadStep{Unmarshalable: make(chan struct{})}},
		},
	}

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)

	assert.Error(t, err)
}

func TestPutManifest_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.WriteFile(filepath.Join(s.basePath, "namespaces"), []byte("block"), 0644))

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{})

	assert.Error(t, err)
}

func TestPutManifest_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{})

	assert.Error(t, err)
}

func TestPutManifest_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, "arrow.json"), 0700))

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{})

	assert.Error(t, err)
}

// deleteManifest tests

func TestDeleteManifest_RemovesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &domain.ArrowManifest{Name: "to-delete"})
	require.NoError(t, err)

	err = deleteManifest(s, ns, "arrow.json")
	require.NoError(t, err)

	_, _, err = getManifest[domain.ArrowManifest](s, ns, "arrow.json")
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestDeleteManifest_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := deleteManifest(s, ns, "arrow.json")

	assert.NoError(t, err)
}
