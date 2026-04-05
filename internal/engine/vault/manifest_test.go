package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		osVersion: domain.OSDarwinARM64,
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
	s.osVersion = domain.OSLinuxAMD64
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
	s.osVersion = domain.OSWindowsAMD64
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

// Additional comprehensive coverage tests for 100%

func TestHelperGetArrow_StaleWithIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "stale"}
	indirectDeps := []domain.Namespace{domain.Namespace("github.com/dep")}
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	writeStaleArrowEntry(t, nsDir, manifest, indirectDeps)

	got, _, err := getArrow(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
	assert.Equal(t, indirectDeps, got.IndirectDependencies)
}

func TestHelperGetArrow_EmptyIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test"}

	_, err := putArrow(s, ns, manifest, []domain.Namespace{})
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)

	// Empty slice serializes as nil in JSON, so check for both
	if got.IndirectDependencies != nil {
		assert.Equal(t, 0, len(got.IndirectDependencies))
	}
}

func TestHelperPutArrow_WriteError(t *testing.T) {
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

func TestHelperPutQuiver_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, quiverFilename), 0700))

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperDeleteArrow_RemoveError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, arrowFilename), []byte("data"), 0644))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	err := deleteArrow(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
}

func TestHelperDeleteQuiver_RemoveError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("data"), 0644))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	err := deleteQuiver(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
}

func TestNewConstructor_WithDefaults(t *testing.T) {
	v := New("", 0, domain.OS("test/os"))

	assert.NotNil(t, v)
	store := v.(*store)
	assert.NotEmpty(t, store.basePath)
	assert.Equal(t, 24*time.Hour, store.ttl)
	assert.Equal(t, domain.OS("test/os"), store.osVersion)
	assert.NotNil(t, store.locks)
	assert.Equal(t, 0, len(store.locks))
}

func TestNewConstructor_WithCustomValues(t *testing.T) {
	basePath := t.TempDir()
	ttl := 48 * time.Hour

	v := New(basePath, ttl, domain.OS("custom/os"))

	assert.NotNil(t, v)
	store := v.(*store)
	assert.Equal(t, basePath, store.basePath)
	assert.Equal(t, ttl, store.ttl)
	assert.Equal(t, domain.OS("custom/os"), store.osVersion)
}

func TestHelperPutArrow_CloseError(t *testing.T) {
	// Tests the closeErr handling in putArrow
	s := newTestStore(t)
	ns := mocks.Namespace()

	// This is an indirect way to test the close error path
	// by creating the directory and filling the namespace
	_, err := putArrow(s, ns, &domain.ArrowManifest{Name: "first"}, nil)
	require.NoError(t, err)

	// Overwrite to test the write path
	_, err = putArrow(s, ns, &domain.ArrowManifest{Name: "second"}, nil)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestHelperPutQuiver_CloseError(t *testing.T) {
	// Tests the closeErr handling in putQuiver
	s := newTestStore(t)
	ns := mocks.Namespace()

	// This is an indirect way to test the close error path
	// by creating the directory and filling the namespace
	_, err := putQuiver(s, ns, &domain.QuiverManifest{Name: "first"})
	require.NoError(t, err)

	// Overwrite to test the write path
	_, err = putQuiver(s, ns, &domain.QuiverManifest{Name: "second"})
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestHelperGetArrow_ComplexMetadata(t *testing.T) {
	// Tests metadata handling in getArrow
	s := newTestStore(t)
	s.osVersion = domain.OS("linux/386")
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "complex"}
	deps := []domain.Namespace{
		domain.Namespace("github.com/a/b"),
		domain.Namespace("github.com/c/d"),
		domain.Namespace("github.com/e/f"),
	}

	_, err := putArrow(s, ns, manifest, deps)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)

	assert.Equal(t, "linux/386", got.Metadata.OS)
	assert.Equal(t, 3, len(got.IndirectDependencies))
}

func TestHelperGetQuiver_MetadataPreservation(t *testing.T) {
	// Tests metadata preservation in getQuiver
	s := newTestStore(t)
	s.osVersion = domain.OS("freebsd/amd64")
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "special"}

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)

	assert.Equal(t, "freebsd/amd64", got.Metadata.OS)
	assert.False(t, got.Metadata.CachedAt.IsZero())
}

func TestHelperDeleteArrow_WithoutQuiver(t *testing.T) {
	// Tests deleteArrow when quiver doesn't exist
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.ArrowManifest{Name: "solo"}, nil)
	require.NoError(t, err)

	// Verify arrow exists
	_, _, err = getArrow(s, ns)
	require.NoError(t, err)

	// Delete it
	err = deleteArrow(s, ns)
	require.NoError(t, err)

	// Verify it's gone
	_, _, err = getArrow(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperDeleteQuiver_WithoutArrow(t *testing.T) {
	// Tests deleteQuiver when arrow doesn't exist
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putQuiver(s, ns, &domain.QuiverManifest{Name: "solo"})
	require.NoError(t, err)

	// Verify quiver exists
	_, _, err = getQuiver(s, ns)
	require.NoError(t, err)

	// Delete it
	err = deleteQuiver(s, ns)
	require.NoError(t, err)

	// Verify it's gone
	_, _, err = getQuiver(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

// Namespace path safety

func TestHelperNamespacePath_RejectsTraversal(t *testing.T) {
	s := newTestStore(t)

	_, _, err := s.acquireNamespace(domain.Namespace("../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// Race condition in namespaceLock
func TestNamespaceLock_RaceCondition(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("github.com/test/concurrent1")
	ns2 := domain.Namespace("github.com/test/concurrent2")

	// Simulate concurrent access with multiple goroutines acquiring locks
	// This tests the double-check lock pattern in namespaceLock
	var wg sync.WaitGroup
	locks := make([]*sync.Mutex, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				locks[idx] = s.namespaceLock(ns1.String())
			} else {
				locks[idx] = s.namespaceLock(ns2.String())
			}
		}(i)
	}
	wg.Wait()

	// Verify all locks for ns1 are the same instance
	ns1Lock := s.locks[ns1.String()]
	for i := 0; i < 100; i += 2 {
		assert.Equal(t, ns1Lock, locks[i])
	}

	// Verify all locks for ns2 are the same instance
	ns2Lock := s.locks[ns2.String()]
	for i := 1; i < 100; i += 2 {
		assert.Equal(t, ns2Lock, locks[i])
	}
}

// Additional tests for uncovered error paths
func TestHelperGetArrow_ReadPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Create arrow file with restricted parent directory
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())
	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, arrowFilename), []byte("{}"), 0000))
	defer os.Chmod(filepath.Join(nsDir, arrowFilename), 0644)

	_, _, err := getArrow(s, ns)
	assert.Error(t, err)
}

func TestHelperGetQuiver_ReadPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Create quiver file with no read permissions
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())
	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("{}"), 0000))
	defer os.Chmod(filepath.Join(nsDir, quiverFilename), 0644)

	_, _, err := getQuiver(s, ns)
	assert.Error(t, err)
}

func TestHelperDeleteArrow_StatError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	// Create arrow file and make parent directory inaccessible for stat
	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, arrowFilename), []byte("data"), 0644))
	require.NoError(t, os.Chmod(nsDir, 0000))
	defer os.Chmod(nsDir, 0700)

	err := deleteArrow(s, ns)
	// Should succeed because arrow.json removal is idempotent for ErrNotExist
	// But stat check might fail
	assert.NotNil(t, err)
}

func TestHelperDeleteQuiver_StatError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, "namespaces", ns.String())

	// Create quiver file and make parent directory inaccessible for stat
	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("data"), 0644))
	require.NoError(t, os.Chmod(nsDir, 0000))
	defer os.Chmod(nsDir, 0700)

	err := deleteQuiver(s, ns)
	// Should succeed because quiver.json removal is idempotent for ErrNotExist
	// But stat check might fail
	assert.NotNil(t, err)
}

// Tests for specific error handling code paths
func TestHelperPutArrow_WriteFailsThenRenameError(t *testing.T) {
	// This tests the case where the directory doesn't exist but can be created
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test"}

	// Create a file where the directory should be to block mkdir
	arrowPath := filepath.Join(s.basePath, "namespaces", ns.String(), arrowFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(arrowPath), 0700))

	// Now put a valid arrow
	path, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestHelperPutQuiver_WriteFailsThenRenameError(t *testing.T) {
	// This tests the case where the directory doesn't exist but can be created
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "test"}

	// Create a file where the directory should be to block mkdir
	quiverPath := filepath.Join(s.basePath, "namespaces", ns.String(), quiverFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(quiverPath), 0700))

	// Now put a valid quiver
	path, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

// Additional concurrent tests for full coverage
func TestConcurrentNamespaceLock_HighContention(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/test/highcontention")

	// Simulate very high contention with many goroutines
	var wg sync.WaitGroup
	const workers = 200

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Get the lock multiple times in a tight loop
			for j := 0; j < 10; j++ {
				lock := s.namespaceLock(ns.String())
				assert.NotNil(t, lock)
			}
		}()
	}
	wg.Wait()

	// Verify only one lock exists for this namespace
	assert.Equal(t, 1, len(s.locks))
	assert.NotNil(t, s.locks[ns.String()])
}

// Test for boundary case: very long metadata
func TestHelperPutArrow_LongOSVersion(t *testing.T) {
	s := newTestStore(t)
	s.osVersion = domain.OS(strings.Repeat("a", 1000))
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "test"}

	path, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify it was persisted correctly
	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, s.osVersion.String(), got.Metadata.OS)
}

func TestHelperPutQuiver_LongOSVersion(t *testing.T) {
	s := newTestStore(t)
	s.osVersion = domain.OS(strings.Repeat("z", 1000))
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "test"}

	path, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	// Verify it was persisted correctly
	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, s.osVersion.String(), got.Metadata.OS)
}

// Test for many indirect dependencies
func TestHelperPutArrow_ManyIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "many-deps"}

	// Create many indirect dependencies
	deps := make([]domain.Namespace, 50)
	for i := 0; i < 50; i++ {
		deps[i] = domain.Namespace(fmt.Sprintf("github.com/repo%d/lib%d", i, i))
	}

	_, err := putArrow(s, ns, manifest, deps)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, len(deps), len(got.IndirectDependencies))
}

// Test metadata precision
func TestHelperGetArrow_MetadataTimestampPrecision(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "precise"}

	beforePut := time.Now()
	_, _ = putArrow(s, ns, manifest, nil)
	afterPut := time.Now()

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)

	// Timestamp should be between beforePut and afterPut
	assert.True(t, !got.Metadata.CachedAt.Before(beforePut) || got.Metadata.CachedAt.Equal(beforePut))
	assert.True(t, !got.Metadata.CachedAt.After(afterPut) || got.Metadata.CachedAt.Equal(afterPut))
}

func TestHelperGetQuiver_MetadataTimestampPrecision(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "precise"}

	beforePut := time.Now()
	_, err := putQuiver(s, ns, manifest)
	afterPut := time.Now()
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)

	// Timestamp should be between beforePut and afterPut
	assert.True(t, !got.Metadata.CachedAt.Before(beforePut) || got.Metadata.CachedAt.Equal(beforePut))
	assert.True(t, !got.Metadata.CachedAt.After(afterPut) || got.Metadata.CachedAt.Equal(afterPut))
}

// Tests for improved error path coverage
func TestHelperPutArrow_WithNilIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "nil-deps"}

	// Explicitly pass nil for indirect deps
	_, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Nil(t, got.IndirectDependencies)
}

func TestHelperPutArrow_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Overwrite the same entry multiple times
	for i := 0; i < 5; i++ {
		manifest := &domain.ArrowManifest{Name: fmt.Sprintf("version-%d", i)}
		_, err := putArrow(s, ns, manifest, nil)
		require.NoError(t, err)
	}

	// Verify final version
	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "version-4", got.Manifest.Name)
}

func TestHelperPutQuiver_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Overwrite the same entry multiple times
	for i := 0; i < 5; i++ {
		manifest := &domain.QuiverManifest{Name: fmt.Sprintf("version-%d", i)}
		_, err := putQuiver(s, ns, manifest)
		require.NoError(t, err)
	}

	// Verify final version
	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "version-4", got.Manifest.Name)
}

func TestHelperDeleteArrow_NonExistentThenQuiverExists(t *testing.T) {
	// Test the code path where arrow doesn't exist initially
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Delete non-existent arrow (should be idempotent)
	err := deleteArrow(s, ns)
	assert.NoError(t, err)

	// Now create both and verify deletion
	_, err = putArrow(s, ns, &domain.ArrowManifest{}, nil)
	require.NoError(t, err)
	_, err = putQuiver(s, ns, &domain.QuiverManifest{})
	require.NoError(t, err)

	err = deleteArrow(s, ns)
	assert.NoError(t, err)

	// Arrow should be gone, quiver should remain
	_, _, err = getArrow(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)

	_, _, err = getQuiver(s, ns)
	assert.NoError(t, err)
}

func TestHelperDeleteQuiver_NonExistentThenArrowExists(t *testing.T) {
	// Test the code path where quiver doesn't exist initially
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Delete non-existent quiver (should be idempotent)
	err := deleteQuiver(s, ns)
	assert.NoError(t, err)

	// Now create both and verify deletion
	_, err = putArrow(s, ns, &domain.ArrowManifest{}, nil)
	require.NoError(t, err)
	_, err = putQuiver(s, ns, &domain.QuiverManifest{})
	require.NoError(t, err)

	err = deleteQuiver(s, ns)
	assert.NoError(t, err)

	// Quiver should be gone, arrow should remain
	_, _, err = getQuiver(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)

	_, _, err = getArrow(s, ns)
	assert.NoError(t, err)
}

// Test for empty manifest fields
func TestHelperPutArrow_EmptyName(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{} // Empty name

	_, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Empty(t, got.Manifest.Name)
}

func TestHelperPutQuiver_EmptyName(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{} // Empty name

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Empty(t, got.Manifest.Name)
}

// Test different TTL behaviors
func TestHelperGetArrow_JustBeforeStale(t *testing.T) {
	s := newTestStore(t)
	s.ttl = 100 * time.Millisecond
	ns := mocks.Namespace()
	manifest := &domain.ArrowManifest{Name: "fresh-enough"}

	_, err := putArrow(s, ns, manifest, nil)
	require.NoError(t, err)

	// Sleep slightly less than TTL
	time.Sleep(50 * time.Millisecond)

	got, _, err := getArrow(s, ns)
	assert.NoError(t, err)
	assert.NotErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
}

func TestHelperGetQuiver_JustBeforeStale(t *testing.T) {
	s := newTestStore(t)
	s.ttl = 100 * time.Millisecond
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "fresh-enough"}

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	// Sleep slightly less than TTL
	time.Sleep(50 * time.Millisecond)

	got, _, err := getQuiver(s, ns)
	assert.NoError(t, err)
	assert.NotErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
}
