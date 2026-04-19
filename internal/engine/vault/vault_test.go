package vault

import (
	"context"
	"errors"
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

func newTestVault(
	t *testing.T,
) Vault {
	return New(t.TempDir(), time.Hour)
}

// GetArrow

func TestGetArrow_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetArrow(context.Background(), mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetArrow_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow)
	require.NoError(t, err)

	got, path, err := v.GetArrow(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, arrow.Name, got.Manifest.Name)
	assert.NotEmpty(t, path)
}

func TestGetArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetArrow(context.Background(), domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
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

	path, err := v.PutArrow(context.Background(), mocks.Namespace(), mocks.ArrowManifest())

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestPutArrow_OverwritesExisting(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "first"}})
	require.NoError(t, err)
	_, err = v.PutArrow(context.Background(), ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "second"}})
	require.NoError(t, err)

	got, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestPutArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutArrow(context.Background(), domain.Namespace(""), mocks.ArrowManifest())

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

// DeleteArrow

func TestDeleteArrow_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest())
	require.NoError(t, err)

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	_, _, err = v.GetArrow(context.Background(), ns)
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
	arrow := mocks.ArrowManifest()
	quiver := mocks.QuiverManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow)
	require.NoError(t, err)
	_, err = v.PutQuiver(context.Background(), ns, quiver)
	require.NoError(t, err)

	gotArrow, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	gotQuiver, _, err := v.GetQuiver(context.Background(), ns)
	require.NoError(t, err)

	assert.Equal(t, arrow.Name, gotArrow.Manifest.Name)
	assert.Equal(t, quiver.Name, gotQuiver.Manifest.Name)
}

// Concurrency

func TestPutArrow_Concurrent(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, arrow)
		}()
	}
	wg.Wait()

	_, _, err := v.GetArrow(context.Background(), ns)
	assert.NoError(t, err)
}

func TestConcurrentGetPutArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.GetArrow(context.Background(), ns)
		}()
		go func() {
			defer wg.Done()
			v.PutArrow(context.Background(), ns, arrow)
		}()
	}
	wg.Wait()
}

func TestConcurrentDeleteGetArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow)
	require.NoError(t, err)

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

// Directory Coexistence

func TestDeleteArrow_RemovesDirectoryWhenEmpty(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest())
	require.NoError(t, err)

	// Get the directory path (without locking, just for testing)
	_, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	// Directory should be gone
	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

// ListVersions

func TestListVersions_ThreeVersions(t *testing.T) {
	v := newTestVault(t)
	bare := domain.Namespace("example.com/user/repo")
	ns1 := domain.Namespace("example.com/user/repo@v1.0.0")
	ns2 := domain.Namespace("example.com/user/repo@v2.0.0")
	ns3 := domain.Namespace("example.com/user/repo@v3.0.0")

	_, err := v.PutArrow(context.Background(), ns1, mocks.ArrowManifest())
	require.NoError(t, err)
	_, err = v.PutArrow(context.Background(), ns2, mocks.ArrowManifest())
	require.NoError(t, err)
	_, err = v.PutArrow(context.Background(), ns3, mocks.ArrowManifest())
	require.NoError(t, err)

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

func TestListVersions_SingleSegmentNamespace_ReturnsEmpty(t *testing.T) {
	// A namespace with no slash → lastSlash < 0 → returns empty
	v := newTestVault(t)

	versions, err := v.ListVersions(context.Background(), domain.Namespace("singlerepo"))

	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestListVersions_DirectoryWithNonDirEntries_SkipsThem(t *testing.T) {
	// Create a file (not directory) in the namespace parent dir; it should be skipped
	v := newTestVault(t)
	ns1 := domain.Namespace("example.com/user/myrepo@v1.0.0")

	_, err := v.PutArrow(context.Background(), ns1, mocks.ArrowManifest())
	require.NoError(t, err)

	// Find parent dir and create a stray file
	bare := domain.Namespace("example.com/user/myrepo")
	s := v.(*store)
	_, parentDir, err := s.acquireNamespace(bare)
	require.NoError(t, err)
	parentDir = parentDir[:len(parentDir)-len("/myrepo")] // go up to user dir

	strayFile := filepath.Join(parentDir, "stray.txt")
	require.NoError(t, os.WriteFile(strayFile, []byte("hi"), 0644))
	t.Cleanup(func() { os.Remove(strayFile) })

	versions, err := v.ListVersions(context.Background(), bare)
	require.NoError(t, err)
	assert.Contains(t, versions, "v1.0.0")
}

func TestListVersions_InvalidNamespace_ReturnsEmpty(t *testing.T) {
	v := newTestVault(t)

	versions, err := v.ListVersions(context.Background(), domain.Namespace(""))

	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestNamespaceLock_SecondCallReturnsSameLock(t *testing.T) {
	v := newTestVault(t)
	s := v.(*store)

	m1 := s.namespaceLock("example.com/user/repo")
	m2 := s.namespaceLock("example.com/user/repo")

	assert.Same(t, m1, m2)
}

func TestDeleteArrow_PreservesDirectoryWhenQuiverExists(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest())
	require.NoError(t, err)
	_, err = v.PutQuiver(context.Background(), ns, mocks.QuiverManifest())
	require.NoError(t, err)

	// Get the directory path (without locking, just for testing)
	_, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	// Directory should still exist (quiver is there)
	_, err = os.Stat(dir)
	assert.NoError(t, err)

	// But arrow.json should be gone
	_, _, err = v.GetArrow(context.Background(), ns)
	assert.ErrorIs(t, err, ErrNotCached)

	// And quiver.json should still be there
	_, _, err = v.GetQuiver(context.Background(), ns)
	assert.NoError(t, err)
}

// DetectLegacyLayout

func TestDetectLegacyLayout_NoLegacy_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	// Create new-style entry: namespace@v1.0.0/arrow.json
	newStyleDir := filepath.Join(
		dir,
		"github.com",
		"org",
		"repo@v1.0.0",
	)
	require.NoError(t, os.MkdirAll(
		newStyleDir,
		0700,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(
			newStyleDir,
			"arrow.json",
		),
		[]byte("{}"),
		0600,
	))

	assert.False(t, DetectLegacyLayout(dir))
}

func TestDetectLegacyLayout_WithLegacy_ReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	// Create old-style entry: no "@" in directory name
	oldStyleDir := filepath.Join(
		dir,
		"github.com",
		"org",
		"repo",
	)
	require.NoError(t, os.MkdirAll(
		oldStyleDir,
		0700,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(
			oldStyleDir,
			"arrow.json",
		),
		[]byte("{}"),
		0600,
	))

	assert.True(t, DetectLegacyLayout(dir))
}

func TestDetectLegacyLayout_EmptyDir_ReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, DetectLegacyLayout(dir))
}
