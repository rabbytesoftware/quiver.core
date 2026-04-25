package vault

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestVault(t *testing.T) Vault {
	return New(t.TempDir(), t.TempDir(), time.Hour)
}

var testManifest = ManifestFile{Content: []byte("# test arrow"), Filename: "ARROW.md"}

// GetArrow

func TestGetArrow_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, err := v.GetArrow(context.Background(), mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetArrow_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	err := v.PutArrow(context.Background(), ns, testManifest)
	require.NoError(t, err)

	got, err := v.GetArrow(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, testManifest.Content, got.Content)
	assert.Equal(t, testManifest.Filename, got.Filename)
}

func TestGetArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.GetArrow(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestGetArrow_Stale(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	base := time.Now()
	v := NewWithClock(vaultDir, nsDir, time.Hour, func() time.Time { return base })

	ns := mocks.Namespace()
	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))

	// Advance clock past TTL
	vWithStaleClock := NewWithClock(vaultDir, nsDir, time.Hour, func() time.Time {
		return base.Add(2 * time.Hour)
	})

	file, err := vWithStaleClock.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrStale)
	// Content still returned on stale
	assert.Equal(t, testManifest.Content, file.Content)
}

// GetQuiver

func TestGetQuiver_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(context.Background(), mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetQuiver_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	quiver := mocks.QuiverManifest()

	_, err := v.PutQuiver(context.Background(), ns, quiver)
	require.NoError(t, err)

	got, path, err := v.GetQuiver(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, quiver.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestGetQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// PutArrow

func TestPutArrow_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	err := v.PutArrow(context.Background(), mocks.Namespace(), testManifest)

	require.NoError(t, err)
}

func TestPutArrow_OverwritesExisting(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	file1 := ManifestFile{Content: []byte("first"), Filename: "ARROW.md"}
	file2 := ManifestFile{Content: []byte("second"), Filename: "ARROW.md"}

	require.NoError(t, v.PutArrow(context.Background(), ns, file1))
	require.NoError(t, v.PutArrow(context.Background(), ns, file2))

	got, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, file2.Content, got.Content)
}

func TestPutArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.PutArrow(context.Background(), domain.Namespace(""), testManifest)

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestPutArrow_CreatesWorkdir(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	v := New(vaultDir, nsDir, time.Hour)
	ns := mocks.Namespace()

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))

	s := v.(*store)
	workdir := s.workdirPath(ns)
	_, err := os.Stat(workdir)
	require.NoError(t, err, "workdir should exist on disk")
}

// WorkDir

func TestWorkDir_CreatesAndReturnsPath(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	dir, err := v.WorkDir(context.Background(), ns)

	require.NoError(t, err)
	assert.NotEmpty(t, dir)
	_, statErr := os.Stat(dir)
	assert.NoError(t, statErr, "workdir should exist on disk after WorkDir call")
}

func TestWorkDir_Idempotent(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	dir1, err := v.WorkDir(context.Background(), ns)
	require.NoError(t, err)
	dir2, err := v.WorkDir(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, dir1, dir2)
}

func TestWorkDir_InvalidNamespace_ReturnsError(t *testing.T) {
	v := newTestVault(t)

	_, err := v.WorkDir(context.Background(), domain.Namespace("../../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// PutQuiver

func TestPutQuiver_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	path, err := v.PutQuiver(context.Background(), mocks.Namespace(), mocks.QuiverManifest())

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestPutQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutQuiver(context.Background(), domain.Namespace(""), mocks.QuiverManifest())

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// DeleteWorkDir

func TestDeleteWorkDir_RemovesDirectoryAndEmptyParents(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace() // "example.com/user/repo"

	dir, err := v.WorkDir(context.Background(), ns)
	require.NoError(t, err)
	require.DirExists(t, dir)

	require.NoError(t, v.DeleteWorkDir(context.Background(), ns))

	assert.NoDirExists(t, dir)
	assert.NoDirExists(t, filepath.Dir(dir))                   // user/
	assert.NoDirExists(t, filepath.Dir(filepath.Dir(dir)))     // example.com/
}

func TestDeleteWorkDir_KeepsNonEmptyParents(t *testing.T) {
	v := newTestVault(t)
	ns1 := domain.Namespace("example.com/user/repo-a")
	ns2 := domain.Namespace("example.com/user/repo-b")

	dir1, err := v.WorkDir(context.Background(), ns1)
	require.NoError(t, err)
	_, err = v.WorkDir(context.Background(), ns2)
	require.NoError(t, err)

	require.NoError(t, v.DeleteWorkDir(context.Background(), ns1))

	// user/ still has repo-b — must not be removed
	assert.NoDirExists(t, dir1)
	assert.DirExists(t, filepath.Dir(dir1)) // user/ still present
}

func TestDeleteWorkDir_Idempotent(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	require.NoError(t, v.DeleteWorkDir(context.Background(), ns))
	require.NoError(t, v.DeleteWorkDir(context.Background(), ns))
}

func TestDeleteWorkDir_InvalidNamespace_ReturnsError(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteWorkDir(context.Background(), domain.Namespace("../../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// DeleteArrow

func TestDeleteArrow_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	_, err := v.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestDeleteArrow_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteArrow(context.Background(), mocks.Namespace()))
}

func TestDeleteArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteArrow(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// DeleteQuiver

func TestDeleteQuiver_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutQuiver(context.Background(), ns, mocks.QuiverManifest())
	require.NoError(t, err)

	require.NoError(t, v.DeleteQuiver(context.Background(), ns))

	_, _, err = v.GetQuiver(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestDeleteQuiver_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteQuiver(context.Background(), mocks.Namespace()))
}

func TestDeleteQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteQuiver(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// Coexistence

func TestArrowAndQuiverCoexist(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	quiver := mocks.QuiverManifest()

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))
	_, err := v.PutQuiver(context.Background(), ns, quiver)
	require.NoError(t, err)

	gotArrow, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	gotQuiver, _, err := v.GetQuiver(context.Background(), ns)
	require.NoError(t, err)

	assert.Equal(t, testManifest.Content, gotArrow.Content)
	assert.Equal(t, quiver.Name, gotQuiver.Manifest.Name)
}

// Concurrency

func TestPutArrow_Concurrent(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, testManifest)
		}()
	}
	wg.Wait()

	_, err := v.GetArrow(context.Background(), ns)
	assert.NoError(t, err)
}

func TestConcurrentGetPutArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.GetArrow(context.Background(), ns)
		}()
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, testManifest)
		}()
	}
	wg.Wait()
}

func TestConcurrentDeleteGetArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	require.NoError(t, v.PutArrow(context.Background(), ns, testManifest))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.DeleteArrow(context.Background(), ns)
		}()
		go func() {
			defer wg.Done()
			v.GetArrow(context.Background(), ns)
		}()
	}
	wg.Wait()
}

// ListVersions

func TestListVersions_ThreeVersions(t *testing.T) {
	v := newTestVault(t)
	bare := domain.Namespace("example.com/user/repo")
	ns1 := domain.Namespace("example.com/user/repo@v1.0.0")
	ns2 := domain.Namespace("example.com/user/repo@v2.0.0")
	ns3 := domain.Namespace("example.com/user/repo@v3.0.0")

	require.NoError(t, v.PutArrow(context.Background(), ns1, testManifest))
	require.NoError(t, v.PutArrow(context.Background(), ns2, testManifest))
	require.NoError(t, v.PutArrow(context.Background(), ns3, testManifest))

	versions, err := v.ListVersions(context.Background(), bare)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"v1.0.0", "v2.0.0", "v3.0.0"}, versions)
}

func TestListVersions_NoVersions(t *testing.T) {
	v := newTestVault(t)
	bare := domain.Namespace("example.com/user/repo")

	versions, err := v.ListVersions(context.Background(), bare)

	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListVersions_NonExistentNamespace(t *testing.T) {
	v := newTestVault(t)

	versions, err := v.ListVersions(context.Background(), domain.Namespace("example.com/nonexistent/repo"))

	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListVersions_InvalidNamespace_ReturnsEmpty(t *testing.T) {
	v := newTestVault(t)

	versions, err := v.ListVersions(context.Background(), domain.Namespace(""))

	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListVersions_SingleSegmentNamespace_ReturnsEmpty(t *testing.T) {
	v := newTestVault(t)

	versions, err := v.ListVersions(context.Background(), domain.Namespace("singlerepo"))

	require.NoError(t, err)
	assert.Empty(t, versions)
}

// NamespaceLock

func TestNamespaceLock_SecondCallReturnsSameLock(t *testing.T) {
	v := newTestVault(t)
	s := v.(*store)

	m1 := s.namespaceLock("example.com/user/repo")
	m2 := s.namespaceLock("example.com/user/repo")

	assert.Same(t, m1, m2)
}

func TestNamespaceLock_DifferentKeysGetDifferentLocks(t *testing.T) {
	v := newTestVault(t)
	s := v.(*store)

	m1 := s.namespaceLock("key1")
	m2 := s.namespaceLock("key2")

	assert.NotSame(t, m1, m2)
}

func TestNamespaceLock_ConcurrentLockCreation(t *testing.T) {
	v := newTestVault(t)
	s := v.(*store)

	const numGoroutines = 200
	key := "race-key"
	results := make(chan *sync.Mutex, numGoroutines)
	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal
			results <- s.namespaceLock(key)
		}()
	}

	close(startSignal)
	wg.Wait()
	close(results)

	var firstLock *sync.Mutex
	for lock := range results {
		if firstLock == nil {
			firstLock = lock
		}
		assert.Same(t, firstLock, lock, "Got different lock instances under race")
	}

	assert.Same(t, firstLock, s.locks[key])
}

// RenameArrow

func TestRenameArrow_MovesFilesAndContents(t *testing.T) {
	v := newTestVault(t)

	oldNs := domain.Namespace("github.com/org/repo@v1.0.0")
	newNs := domain.Namespace("github.com/org/repo@v2.0.0")

	require.NoError(t, v.PutArrow(context.Background(), oldNs, testManifest))

	err := v.RenameArrow(context.Background(), oldNs, newNs)
	require.NoError(t, err)

	_, err = v.GetArrow(context.Background(), oldNs)
	assert.ErrorIs(t, err, ErrNotCached)

	got, err := v.GetArrow(context.Background(), newNs)
	require.NoError(t, err)
	assert.Equal(t, testManifest.Content, got.Content)
}

func TestRenameArrow_SameNamespace_Noop(t *testing.T) {
	v := newTestVault(t)
	ns := domain.Namespace("github.com/org/repo@v1.0.0")

	err := v.RenameArrow(context.Background(), ns, ns)
	require.NoError(t, err)
}

func TestRenameArrow_SourceDoesNotExist(t *testing.T) {
	v := newTestVault(t)

	oldNs := domain.Namespace("github.com/org/nonexistent@v1.0.0")
	newNs := domain.Namespace("github.com/org/new@v1.0.0")

	err := v.RenameArrow(context.Background(), oldNs, newNs)
	assert.Error(t, err)
}

func TestRenameArrow_WithInvalidOldNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.RenameArrow(context.Background(), domain.Namespace(""), domain.Namespace("github.com/org/new@v1.0.0"))
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestRenameArrow_WithInvalidNewNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.RenameArrow(context.Background(), domain.Namespace("github.com/org/repo@v1.0.0"), domain.Namespace(""))
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}
