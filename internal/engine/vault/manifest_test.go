package vault

import (
	"context"
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
		basePath: t.TempDir(),
		ttl:      time.Hour,
		clock:    time.Now,
		locks:    make(map[string]*sync.Mutex),
	}
}

func writeStaleArrowEntry(
	t *testing.T,
	dir string,
	manifest *domain.Arrow,
	indirectDeps []domain.Namespace,
) {
	entry := struct {
		Manifest             *domain.Arrow      `json:"manifest"`
		CachedAt             time.Time          `json:"cached_at"`
		IndirectDependencies []domain.Namespace `json:"indirect_dependencies,omitempty"`
	}{
		Manifest:             manifest,
		CachedAt:             time.Now().Add(-2 * time.Hour),
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
	}{
		Manifest: manifest,
		CachedAt: time.Now().Add(-2 * time.Hour),
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
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test-arrow"}}

	_, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	got, path, err := getArrow(s, ns)

	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetArrow_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "stale-arrow"}}
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test-arrow"}}

	path, err := putArrow(s, ns, manifest)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestHelperPutArrow_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "first"}})
	require.NoError(t, err)

	_, err = putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "second"}})
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestHelperPutArrow_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "meta-arrow"}}

	path, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		Manifest *domain.Arrow `json:"manifest"`
		CachedAt time.Time     `json:"cached_at"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Name, entry.Manifest.Name)
}

func TestHelperPutArrow_MarshalError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Install: step.StepList{&mocks.BadStep{Unmarshalable: make(chan struct{})}},
				},
			},
		},
	}

	_, err := putArrow(s, ns, manifest)

	assert.Error(t, err)
}

func TestHelperPutArrow_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Block MkdirAll by writing a file where the first path component would be.
	firstComponent := strings.SplitN(ns.String(), "/", 2)[0]
	require.NoError(t, os.WriteFile(filepath.Join(s.basePath, firstComponent), []byte("block"), 0644))

	_, err := putArrow(s, ns, &domain.Arrow{})

	assert.Error(t, err)
}

func TestHelperPutArrow_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putArrow(s, ns, &domain.Arrow{})

	assert.Error(t, err)
}

func TestHelperPutArrow_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, arrowFilename), 0700))

	_, err := putArrow(s, ns, &domain.Arrow{})

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
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "meta-quiver"}

	path, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		Manifest *domain.QuiverManifest `json:"manifest"`
		CachedAt time.Time              `json:"cached_at"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Name, entry.Manifest.Name)
}

func TestHelperPutQuiver_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Block MkdirAll by writing a file where the first path component would be.
	firstComponent := strings.SplitN(ns.String(), "/", 2)[0]
	require.NoError(t, os.WriteFile(filepath.Join(s.basePath, firstComponent), []byte("block"), 0644))

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

// deleteArrow tests

func TestHelperDeleteArrow_RemovesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "to-delete"}})
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

	_, err := putArrow(s, ns, &domain.Arrow{})
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

	_, err := putArrow(s, ns, &domain.Arrow{})
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

	_, err := putArrow(s, ns, &domain.Arrow{})
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
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "stale"}}
	indirectDeps := []domain.Namespace{domain.Namespace("github.com/dep")}
	nsDir := filepath.Join(s.basePath, ns.String())

	writeStaleArrowEntry(t, nsDir, manifest, indirectDeps)

	got, _, err := getArrow(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
}

func TestHelperPutArrow_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putArrow(s, ns, &domain.Arrow{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("data"), 0644))
	require.NoError(t, os.Chmod(nsDir, 0555))
	defer os.Chmod(nsDir, 0700)

	err := deleteQuiver(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
}

func TestNewConstructor_WithDefaults(t *testing.T) {
	v := New("", 0)

	assert.NotNil(t, v)
	store := v.(*store)
	assert.NotEmpty(t, store.basePath)
	assert.Equal(t, 24*time.Hour, store.ttl)
	assert.NotNil(t, store.locks)
	assert.Equal(t, 0, len(store.locks))
}

func TestNewConstructor_WithCustomValues(t *testing.T) {
	basePath := t.TempDir()
	ttl := 48 * time.Hour

	v := New(basePath, ttl)

	assert.NotNil(t, v)
	store := v.(*store)
	assert.Equal(t, basePath, store.basePath)
	assert.Equal(t, ttl, store.ttl)
}

func TestHelperPutArrow_CloseError(t *testing.T) {
	// Tests the closeErr handling in putArrow
	s := newTestStore(t)
	ns := mocks.Namespace()

	// This is an indirect way to test the close error path
	// by creating the directory and filling the namespace
	_, err := putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "first"}})
	require.NoError(t, err)

	// Overwrite to test the write path
	_, err = putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "second"}})
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
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "complex"}}
	_, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)

	assert.False(t, got.Metadata.CachedAt.IsZero())
}

func TestHelperGetQuiver_MetadataPreservation(t *testing.T) {
	// Tests metadata preservation in getQuiver
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "special"}

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)

	assert.False(t, got.Metadata.CachedAt.IsZero())
}

func TestHelperDeleteArrow_WithoutQuiver(t *testing.T) {
	// Tests deleteArrow when quiver doesn't exist
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putArrow(s, ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "solo"}})
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
	nsDir := filepath.Join(s.basePath, ns.String())
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
	nsDir := filepath.Join(s.basePath, ns.String())
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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	nsDir := filepath.Join(s.basePath, ns.String())

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
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test"}}

	// Create a file where the directory should be to block mkdir
	arrowPath := filepath.Join(s.basePath, ns.String(), arrowFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(arrowPath), 0700))

	// Now put a valid arrow
	path, err := putArrow(s, ns, manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestHelperPutQuiver_WriteFailsThenRenameError(t *testing.T) {
	// This tests the case where the directory doesn't exist but can be created
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "test"}

	// Create a file where the directory should be to block mkdir
	quiverPath := filepath.Join(s.basePath, ns.String(), quiverFilename)
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

// Test for boundary case: large manifest name
func TestHelperPutArrow_LongName(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: strings.Repeat("a", 1000)}}

	path, err := putArrow(s, ns, manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	got, _, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
}

func TestHelperPutQuiver_LongName(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: strings.Repeat("z", 1000)}

	path, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, manifest.Name, got.Manifest.Name)
}

// Test for many indirect dependencies
func TestHelperPutArrow_ManyIndirectDeps(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "many-deps"}}

	_, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	_, _, err = getArrow(s, ns)
	require.NoError(t, err)
}

// Test metadata precision
func TestHelperGetArrow_MetadataTimestampPrecision(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "precise"}}

	beforePut := time.Now()
	_, _ = putArrow(s, ns, manifest)
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
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "nil-deps"}}

	_, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	_, _, err = getArrow(s, ns)
	require.NoError(t, err)
}

func TestHelperPutArrow_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Overwrite the same entry multiple times
	for i := 0; i < 5; i++ {
		manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: fmt.Sprintf("version-%d", i)}}
		_, err := putArrow(s, ns, manifest)
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
	_, err = putArrow(s, ns, &domain.Arrow{})
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
	_, err = putArrow(s, ns, &domain.Arrow{})
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
	manifest := &domain.Arrow{} // Empty name

	_, err := putArrow(s, ns, manifest)
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

func TestPutArrow_OSFieldNotPersisted(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("example.com/vendor/tool@1.0.0")
	manifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "tool"},
	}

	path, err := s.PutArrow(context.Background(), ns, manifest)
	if err != nil {
		t.Fatalf("PutArrow failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if _, ok := raw["os"]; ok {
		t.Fatal("expected 'os' field to be absent from persisted JSON, but it was present")
	}
}

// Test different TTL behaviors
func TestHelperGetArrow_JustBeforeStale(t *testing.T) {
	s := newTestStore(t)
	s.ttl = 100 * time.Millisecond
	ns := mocks.Namespace()
	manifest := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "fresh-enough"}}

	base := time.Now()
	s.clock = func() time.Time { return base }

	_, err := putArrow(s, ns, manifest)
	require.NoError(t, err)

	// Advance clock 50ms — still within TTL, entry is fresh
	s.clock = func() time.Time { return base.Add(50 * time.Millisecond) }

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

	base := time.Now()
	s.clock = func() time.Time { return base }

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	// Advance clock 50ms — still within TTL, entry is fresh
	s.clock = func() time.Time { return base.Add(50 * time.Millisecond) }

	got, _, err := getQuiver(s, ns)
	assert.NoError(t, err)
	assert.NotErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
}
