package vault

import (
	"context"
	"errors"
	"os"
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
	return New(t.TempDir(), time.Hour, "darwin/arm64")
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

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
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

	path, err := v.PutArrow(context.Background(), mocks.Namespace(), mocks.ArrowManifest(), nil)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestPutArrow_OverwritesExisting(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, &domain.ArrowManifest{Name: "first"}, nil)
	require.NoError(t, err)
	_, err = v.PutArrow(context.Background(), ns, &domain.ArrowManifest{Name: "second"}, nil)
	require.NoError(t, err)

	got, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Manifest.Name)
}

func TestPutArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutArrow(context.Background(), domain.Namespace(""), mocks.ArrowManifest(), nil)

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

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
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

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
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
			v.PutArrow(context.Background(), ns, arrow, nil)
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

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
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
			v.PutArrow(context.Background(), ns, arrow, nil)
		}()
	}
	wg.Wait()
}

func TestConcurrentDeleteGetArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(context.Background(), ns, arrow, nil)
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

// IndirectDeps and Directory Coexistence

func TestPutArrow_PersistsIndirectDeps(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()
	indirectDeps := []domain.Namespace{
		domain.Namespace("github.com/foo/bar"),
		domain.Namespace("github.com/baz/qux"),
	}

	_, err := v.PutArrow(context.Background(), ns, arrow, indirectDeps)
	require.NoError(t, err)

	got, _, err := v.GetArrow(context.Background(), ns)
	require.NoError(t, err)

	assert.Equal(t, indirectDeps, got.IndirectDependencies)
}

func TestDeleteArrow_RemovesDirectoryWhenEmpty(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
	require.NoError(t, err)

	// Get the directory path
	mu, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)
	mu.Lock()
	mu.Unlock()

	require.NoError(t, v.DeleteArrow(context.Background(), ns))

	// Directory should be gone
	_, err = os.Stat(dir)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestDeleteArrow_PreservesDirectoryWhenQuiverExists(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(context.Background(), ns, mocks.ArrowManifest(), nil)
	require.NoError(t, err)
	_, err = v.PutQuiver(context.Background(), ns, mocks.QuiverManifest())
	require.NoError(t, err)

	// Get the directory path
	mu, dir, err := v.(*store).acquireNamespace(ns)
	require.NoError(t, err)
	mu.Lock()
	mu.Unlock()

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
