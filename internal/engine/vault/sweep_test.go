package vault

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestSweep_StaleArrow_DeletesFiles(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

	ns := domain.Namespace("github.com/org/quiver@v1")
	_, err = s.PutCollection(context.Background(), ns, &domain.Collection{})
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

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

	ns := domain.Namespace("github.com/org/quiver@v1")
	_, err = s.PutCollection(context.Background(), ns, &domain.Collection{})
	require.NoError(t, err)

	s.clock = fixedClock(now.Add(ttl - time.Second))
	s.sweep()

	quiverFile := filepath.Join(nsDir, filepath.FromSlash(string(ns)), quiverFilename)
	assert.FileExists(t, quiverFile)
}

func TestSweep_EmptyVaultDir_NoError(t *testing.T) {
	v, err := NewWithClock(t.TempDir(), t.TempDir(), time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)
	assert.NotPanics(t, s.sweep)
}

func TestSweep_NonexistentVaultDir_NoError(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")
	nsDir := filepath.Join(dir, "ns")

	v, err := NewWithClock(vaultDir, nsDir, time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	// The constructor creates the vault dir; remove it again so sweep runs
	// against directories that are gone.
	require.NoError(t, os.RemoveAll(vaultDir))

	s := v.(*store)
	assert.NotPanics(t, s.sweep)
}

func TestNewWithClock_UncreatableVaultDir_Error(t *testing.T) {
	// A file where the vault directory should be — MkdirAll cannot succeed.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	_, err := NewWithClock(filepath.Join(blocker, "vault"), t.TempDir(), time.Hour, time.Now)
	require.ErrorContains(t, err, "vault: create dir")
}

func TestNewWithClock_UnopenableIndex_Error(t *testing.T) {
	// A directory named index.db blocks SQLite from creating the database file.
	vaultDir := filepath.Join(t.TempDir(), "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, indexFilename), 0o700))

	_, err := NewWithClock(vaultDir, t.TempDir(), time.Hour, time.Now)
	require.ErrorContains(t, err, "vault index")
}

// Ensure sweep removes the arrow manifest file (not just the meta sidecar).
func TestSweep_StaleArrow_RemovesManifestFile(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	now := time.Now()
	ttl := time.Hour

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

	ns := domain.Namespace("github.com/org/tool@v1")
	require.NoError(t, s.PutArrow(context.Background(), ns, ManifestFile{Content: []byte("data"), Filename: "arrow.yaml"}))

	s.clock = fixedClock(now.Add(ttl + time.Second))
	s.sweep()

	entries, err := os.ReadDir(vaultDir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.True(t, strings.HasPrefix(e.Name(), indexFilename),
			"only the read-model database may survive a sweep, found %s", e.Name())
	}
}

func TestSweepArrows_MalformedFilename_Skips(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	v, err := NewWithClock(vaultDir, nsDir, time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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
	v, err := NewWithClock(vaultDir, nsDir, time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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
	v, err := NewWithClock(vaultDir, nsDir, time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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

	v, err := NewWithClock(vaultDir, nsDir, ttl, fixedClock(now))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

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

func TestSweep_ByteExpiryKeepsIndexRow(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	v, err := NewWithClock(
		filepath.Join(dir, "vault"),
		filepath.Join(dir, "ns"),
		24*time.Hour,
		func() time.Time { return now },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	meta := IndexMeta{Arrow: domain.ArrowMeta{Name: "Chromium"}}
	require.NoError(t, v.PutArrow(context.Background(), "github.com/u/r@v1", ManifestFile{
		Content: []byte("x"), Filename: "ARROW.md", Meta: &meta,
	}))

	// Past the 24h byte TTL, well inside the 30d row TTL.
	now = now.Add(48 * time.Hour)
	v.(*store).sweep()

	_, err = v.GetArrow(context.Background(), "github.com/u/r@v1")
	require.ErrorIs(t, err, ErrNotCached, "bytes must be gone")

	rows, err := v.SearchArrows(context.Background(), IndexQuery{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1, "the row must outlive its bytes")
}

func TestSweep_RowExpiryRemovesIndexRow(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	v, err := NewWithClock(
		filepath.Join(dir, "vault"),
		filepath.Join(dir, "ns"),
		24*time.Hour,
		func() time.Time { return now },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	meta := IndexMeta{Arrow: domain.ArrowMeta{Name: "Chromium"}}
	require.NoError(t, v.PutArrow(context.Background(), "github.com/u/r@v1", ManifestFile{
		Content: []byte("x"), Filename: "ARROW.md", Meta: &meta,
	}))

	now = now.Add(31 * 24 * time.Hour)
	v.(*store).sweep()

	rows, err := v.SearchArrows(context.Background(), IndexQuery{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestSweep_IndexEvictionError_DoesNotPanic(t *testing.T) {
	v, err := NewWithClock(t.TempDir(), t.TempDir(), time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)
	require.NoError(t, s.idx.db.Exec(`DROP TABLE vault_arrows`).Error)

	assert.NotPanics(t, s.sweep)
}

func TestSweepArrows_VaultPathIsFile_NoPanic(t *testing.T) {
	v, err := NewWithClock(t.TempDir(), t.TempDir(), time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)

	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	s.vaultPath = blocker

	assert.NotPanics(t, s.sweepArrows)
}

func TestReadQuiverCachedAt_MissingFile_Error(t *testing.T) {
	_, err := readQuiverCachedAt(filepath.Join(t.TempDir(), "nope.json"))

	assert.Error(t, err)
}

// A sweep that started before the vault closed finds the index gone. That is
// shutdown, not a failure: the rows it would have evicted go with the process.
func TestSweep_AfterClose_IsSilentAndSafe(t *testing.T) {
	v, err := NewWithClock(t.TempDir(), t.TempDir(), time.Hour, time.Now)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	s := v.(*store)
	require.NoError(t, s.Close())

	assert.NotPanics(t, s.sweep)
}
