package paths_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ensureCreatesDir verifies that calling fn creates the directory on disk.
func ensureCreatesDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	got, err := fn()
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr, "directory should exist after call")
	assert.True(t, info.IsDir())
}

func TestEvents_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Events)
}

func TestEvents_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Events()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestStore_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Store)
}

func TestStore_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Store()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestNamespaces_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Namespaces)
}

func TestNamespaces_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Namespaces()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestLogs_CreatesDir(t *testing.T) {
	ensureCreatesDir(t, paths.Logs)
}

func TestLogs_ReturnsAbsolutePath(t *testing.T) {
	got, err := paths.Logs()
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(got))
}

func TestEvents_Idempotent(t *testing.T) {
	first, err := paths.Events()
	require.NoError(t, err)
	second, err := paths.Events()
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestConcurrentCalls_NoRace(t *testing.T) {
	// Run with -race to detect data races.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = paths.Events()
			_, _ = paths.Store()
			_, _ = paths.Namespaces()
			_, _ = paths.Logs()
		}()
	}
	wg.Wait()
}

func TestEventsAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := paths.EventsAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Contains(t, got, home)
}

func TestStoreAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := paths.StoreAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Contains(t, got, home)
}

func TestNamespacesAt_CreatesDir(t *testing.T) {
	home := t.TempDir()
	got, err := paths.NamespacesAt(home)
	require.NoError(t, err)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
	assert.Contains(t, got, home)
}

func TestEnsure_MkdirAllError_ReturnsError(t *testing.T) {
	// Create a regular file, then try to mkdir under it — guaranteed to fail.
	f, err := os.CreateTemp("", "paths-test-*")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	badPath := filepath.Join(f.Name(), "subdir")
	_, gotErr := paths.Ensure(badPath)
	assert.Error(t, gotErr)
}
