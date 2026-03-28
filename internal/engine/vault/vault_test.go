package vault

import (
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

	_, _, err := v.GetArrow(mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetArrow_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(ns, arrow)
	require.NoError(t, err)

	got, path, err := v.GetArrow(ns)

	require.NoError(t, err)
	assert.Equal(t, arrow.Name, got.Name)
	assert.NotEmpty(t, path)
}

func TestGetArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetArrow(domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// GetQuiver

func TestGetQuiver_NotCached(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestGetQuiver_Fresh(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	quiver := mocks.QuiverManifest()

	_, err := v.PutQuiver(ns, quiver)
	require.NoError(t, err)

	got, path, err := v.GetQuiver(ns)

	require.NoError(t, err)
	assert.Equal(t, quiver.Name, got.Name)
	assert.NotEmpty(t, path)
}

func TestGetQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, _, err := v.GetQuiver(domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// PutArrow

func TestPutArrow_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	path, err := v.PutArrow(mocks.Namespace(), mocks.ArrowManifest())

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestPutArrow_OverwritesExisting(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(ns, &domain.ArrowManifest{Name: "first"})
	require.NoError(t, err)
	_, err = v.PutArrow(ns, &domain.ArrowManifest{Name: "second"})
	require.NoError(t, err)

	got, _, err := v.GetArrow(ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Name)
}

func TestPutArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutArrow(domain.Namespace(""), mocks.ArrowManifest())

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// PutQuiver

func TestPutQuiver_CreatesFile(t *testing.T) {
	v := newTestVault(t)

	path, err := v.PutQuiver(mocks.Namespace(), mocks.QuiverManifest())

	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestPutQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	_, err := v.PutQuiver(domain.Namespace(""), mocks.QuiverManifest())

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// DeleteArrow

func TestDeleteArrow_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutArrow(ns, mocks.ArrowManifest())
	require.NoError(t, err)

	require.NoError(t, v.DeleteArrow(ns))

	_, _, err = v.GetArrow(ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestDeleteArrow_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteArrow(mocks.Namespace()))
}

func TestDeleteArrow_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteArrow(domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// DeleteQuiver

func TestDeleteQuiver_RemovesFile(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()

	_, err := v.PutQuiver(ns, mocks.QuiverManifest())
	require.NoError(t, err)

	require.NoError(t, v.DeleteQuiver(ns))

	_, _, err = v.GetQuiver(ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestDeleteQuiver_Idempotent(t *testing.T) {
	v := newTestVault(t)

	assert.NoError(t, v.DeleteQuiver(mocks.Namespace()))
}

func TestDeleteQuiver_InvalidNamespace(t *testing.T) {
	v := newTestVault(t)

	err := v.DeleteQuiver(domain.Namespace(""))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// Coexistence

func TestArrowAndQuiverCoexist(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()
	quiver := mocks.QuiverManifest()

	_, err := v.PutArrow(ns, arrow)
	require.NoError(t, err)
	_, err = v.PutQuiver(ns, quiver)
	require.NoError(t, err)

	gotArrow, _, err := v.GetArrow(ns)
	require.NoError(t, err)
	gotQuiver, _, err := v.GetQuiver(ns)
	require.NoError(t, err)

	assert.Equal(t, arrow.Name, gotArrow.Name)
	assert.Equal(t, quiver.Name, gotQuiver.Name)
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
			v.PutArrow(ns, arrow)
		}()
	}
	wg.Wait()

	_, _, err := v.GetArrow(ns)
	assert.NoError(t, err)
}

func TestConcurrentGetPutArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(ns, arrow)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.GetArrow(ns)
		}()
		go func() {
			defer wg.Done()
			v.PutArrow(ns, arrow)
		}()
	}
	wg.Wait()
}

func TestConcurrentDeleteGetArrow(t *testing.T) {
	v := newTestVault(t)
	ns := mocks.Namespace()
	arrow := mocks.ArrowManifest()

	_, err := v.PutArrow(ns, arrow)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			v.DeleteArrow(ns)
		}()
		go func() {
			defer wg.Done()
			v.GetArrow(ns)
		}()
	}
	wg.Wait()
}
