package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestSweep_StaleArrow_DeletesFiles(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/tool@v1")
	require.NoError(t, s.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("data"), Filename: "arrow.yaml"}))

	s.clock = fixedClock(now.Add(ttl + time.Second))
	s.sweep()

	assert.NoFileExists(t, s.metaFilePath(ns))
	assert.NoFileExists(t, s.manifestFilePath(ns, "arrow.yaml"))
}

func TestSweep_FreshArrow_PreservesFiles(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/tool@v1")
	require.NoError(t, s.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("data"), Filename: "arrow.yaml"}))

	s.clock = fixedClock(now.Add(ttl - time.Second))
	s.sweep()

	assert.FileExists(t, s.metaFilePath(ns))
}

func TestSweep_StaleQuiver_DeletesFile(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/quiver@v1")
	_, err := s.PutCollection(context.Background(), ns, &domain.Collection{})
	require.NoError(t, err)

	quiverFile := filepath.Join(nsDir, filepath.FromSlash(string(ns)), quiverFilename)
	require.FileExists(t, quiverFile)

	s.clock = fixedClock(now.Add(ttl + time.Second))
	s.sweep()

	assert.NoFileExists(t, quiverFile)
}

func TestSweep_FreshQuiver_PreservesFile(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/quiver@v1")
	_, err := s.PutCollection(context.Background(), ns, &domain.Collection{})
	require.NoError(t, err)

	s.clock = fixedClock(now.Add(ttl - time.Second))
	s.sweep()

	quiverFile := filepath.Join(nsDir, filepath.FromSlash(string(ns)), quiverFilename)
	assert.FileExists(t, quiverFile)
}

func TestSweep_EmptyVaultDir_NoError(t *testing.T) {
	s := NewWithClock(t.TempDir(), t.TempDir(), time.Hour, time.Now).(*store)
	assert.NotPanics(t, s.sweep)
}

func TestSweep_NonexistentVaultDir_NoError(t *testing.T) {
	s := NewWithClock("/nonexistent/vault", "/nonexistent/ns", time.Hour, time.Now).(*store)
	assert.NotPanics(t, s.sweep)
}

// Ensure sweep removes the arrow manifest file (not just the meta sidecar).
func TestSweep_StaleArrow_RemovesManifestFile(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/tool@v1")
	require.NoError(t, s.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("data"), Filename: "arrow.yaml"}))

	s.clock = fixedClock(now.Add(ttl + time.Second))
	s.sweep()

	entries, err := os.ReadDir(vaultDir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSweepArrows_MalformedFilename_Skips(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	s := NewWithClock(vaultDir, nsDir, time.Hour, time.Now).(*store)

	// Write a meta file with an invalid percent-encoded name.
	// "%ZZ" is not valid URL encoding and url.PathUnescape will return an error.
	badName := filepath.Join(vaultDir, "%ZZ.meta.json")
	require.NoError(t, os.WriteFile(badName, []byte(`{"cached_at":"2000-01-01T00:00:00Z","filename":"arrow.yaml"}`), 0o600))

	// Should not panic.
	assert.NotPanics(t, s.sweep)
	// The bad file should still be there (skipped, not deleted).
	assert.FileExists(t, badName)
}

func TestSweepQuivers_CorruptedJSON_Skips(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	s := NewWithClock(vaultDir, nsDir, time.Hour, time.Now).(*store)

	// Create a quiver directory with a malformed quiver.json.
	ns := domain.Namespace("github.com/org/broken@v1")
	dir := filepath.Join(nsDir, filepath.FromSlash(string(ns)))
	require.NoError(t, os.MkdirAll(dir, 0o700))
	quiverPath := filepath.Join(dir, quiverFilename)
	require.NoError(t, os.WriteFile(quiverPath, []byte(`not json`), 0o600))

	// Should not panic, corrupted file is skipped.
	assert.NotPanics(t, s.sweep)
	// The corrupted file should still be there (skipped, not deleted).
	assert.FileExists(t, quiverPath)
}

func TestSweepArrows_CorruptedMeta_Skips(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	s := NewWithClock(vaultDir, nsDir, time.Hour, time.Now).(*store)

	// Write a valid-looking meta filename but with invalid JSON content.
	ns := domain.Namespace("github.com/org/tool@v1")
	metaPath := s.metaFilePath(ns)
	require.NoError(t, os.WriteFile(metaPath, []byte(`{bad json}`), 0o600))

	// Should not panic, broken meta is skipped.
	assert.NotPanics(t, s.sweep)
	// The broken meta file should still be there.
	assert.FileExists(t, metaPath)
}

func TestStart_SweepFiresAfterInterval(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	s := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now)).(*store)

	ns := domain.Namespace("github.com/org/tool@v1")
	require.NoError(t, s.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("d"), Filename: "arrow.yaml"}))
	s.clock = fixedClock(now.Add(ttl + time.Second))

	s.sweepInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	require.Eventually(t, func() bool {
		_, err := os.Stat(s.metaFilePath(ns))
		return errors.Is(err, os.ErrNotExist)
	}, time.Second, 5*time.Millisecond)
}
