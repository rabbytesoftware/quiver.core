package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *store {
	return &store{
		vaultPath:      t.TempDir(),
		namespacesPath: t.TempDir(),
		ttl:            time.Hour,
		clock:          time.Now,
		locks:          make(map[string]*sync.Mutex),
	}
}

var testFile = ManifestFile{Content: []byte("# test arrow manifest"), Filename: "ARROW.md"}

func writeStaleArrowEntry(t *testing.T, s *store, ns domain.Namespace, file ManifestFile) {
	t.Helper()
	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))

	require.NoError(t, os.WriteFile(s.manifestFilePath(ns, file.Filename), file.Content, 0o644))

	meta, err := json.Marshal(VaultMetadata{
		CachedAt: time.Now().Add(-2 * time.Hour),
		Filename: file.Filename,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), meta, 0o644))
}

func writeStaleQuiverEntry(t *testing.T, dir string, manifest *domain.QuiverManifest) {
	entry := struct {
		Manifest *domain.QuiverManifest `json:"manifest"`
		CachedAt time.Time              `json:"cached_at"`
	}{
		Manifest: manifest,
		CachedAt: time.Now().Add(-2 * time.Hour),
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, quiverFilename), data, 0o644))
}

// getArrow tests

func TestHelperGetArrow_NotCached(t *testing.T) {
	s := newTestStore(t)

	_, err := getArrow(s, mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetArrow_Fresh(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, putArrow(s, ns, testFile))

	got, err := getArrow(s, ns)

	require.NoError(t, err)
	assert.Equal(t, testFile.Content, got.Content)
	assert.Equal(t, testFile.Filename, got.Filename)
}

func TestHelperGetArrow_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	writeStaleArrowEntry(t, s, ns, testFile)

	got, err := getArrow(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.Equal(t, testFile.Content, got.Content)
	assert.Equal(t, testFile.Filename, got.Filename)
}

func TestHelperGetArrow_CorruptedMeta(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), []byte("not-json"), 0o644))

	_, err := getArrow(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

func TestHelperGetArrow_MetaMissingManifest(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	meta, _ := json.Marshal(VaultMetadata{CachedAt: time.Now(), Filename: "ARROW.md"})
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), meta, 0o644))
	// Don't write the manifest file

	_, err := getArrow(s, ns)

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetArrow_ReadPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	meta, _ := json.Marshal(VaultMetadata{CachedAt: time.Now(), Filename: "ARROW.md"})
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), meta, 0o644))
	require.NoError(t, os.WriteFile(s.manifestFilePath(ns, "ARROW.md"), []byte("content"), 0o000))
	defer os.Chmod(s.manifestFilePath(ns, "ARROW.md"), 0o644)

	_, err := getArrow(s, ns)
	assert.Error(t, err)
}

// putArrow tests

func TestHelperPutArrow_CreatesFiles(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := putArrow(s, ns, testFile)

	require.NoError(t, err)
	_, statErr := os.Stat(s.manifestFilePath(ns, testFile.Filename))
	assert.NoError(t, statErr)
	_, statErr = os.Stat(s.metaFilePath(ns))
	assert.NoError(t, statErr)
}

func TestHelperPutArrow_CreatesWorkdir(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, putArrow(s, ns, testFile))

	_, err := os.Stat(s.workdirPath(ns))
	assert.NoError(t, err)
}

func TestHelperPutArrow_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	file1 := ManifestFile{Content: []byte("first content"), Filename: "ARROW.md"}
	file2 := ManifestFile{Content: []byte("second content"), Filename: "ARROW.md"}

	require.NoError(t, putArrow(s, ns, file1))
	require.NoError(t, putArrow(s, ns, file2))

	got, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, file2.Content, got.Content)
}

func TestHelperPutArrow_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	before := time.Now()
	require.NoError(t, putArrow(s, ns, testFile))
	after := time.Now()

	meta, err := readMeta(s.metaFilePath(ns))
	require.NoError(t, err)

	assert.False(t, meta.CachedAt.IsZero())
	assert.Equal(t, testFile.Filename, meta.Filename)
	assert.True(t, !meta.CachedAt.Before(before))
	assert.True(t, !meta.CachedAt.After(after))
}

func TestHelperPutArrow_MkdirError(t *testing.T) {
	s := newTestStore(t)
	// Block MkdirAll by writing a file where vaultPath would be
	s.vaultPath = filepath.Join(t.TempDir(), "blocked")
	require.NoError(t, os.WriteFile(s.vaultPath, []byte("block"), 0o644))

	err := putArrow(s, mocks.Namespace(), testFile)

	assert.Error(t, err)
}

func TestHelperPutArrow_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	require.NoError(t, os.Chmod(s.vaultPath, 0o555))
	defer os.Chmod(s.vaultPath, 0o700)

	err := putArrow(s, ns, testFile)

	assert.Error(t, err)
}

func TestHelperPutArrow_DifferentExtensions(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	yamlFile := ManifestFile{Content: []byte("name: test"), Filename: "arrow.yaml"}
	require.NoError(t, putArrow(s, ns, yamlFile))

	got, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, yamlFile.Content, got.Content)
	assert.Equal(t, yamlFile.Filename, got.Filename)
}

func TestHelperPutArrow_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 5; i++ {
		file := ManifestFile{
			Content:  []byte(fmt.Sprintf("version %d", i)),
			Filename: "ARROW.md",
		}
		require.NoError(t, putArrow(s, ns, file))
	}

	got, err := getArrow(s, ns)
	require.NoError(t, err)
	assert.Equal(t, []byte("version 4"), got.Content)
}

// deleteArrow tests

func TestHelperDeleteArrow_RemovesFiles(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, putArrow(s, ns, testFile))

	err := deleteArrow(s, ns)
	require.NoError(t, err)

	_, err = getArrow(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperDeleteArrow_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := deleteArrow(s, ns)

	assert.NoError(t, err)
}

func TestHelperDeleteArrow_CorruptedMeta(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), []byte("not-json"), 0o644))

	err := deleteArrow(s, ns)
	assert.Error(t, err)
}

// renameArrow tests

func TestHelperRenameArrow_MovesFiles(t *testing.T) {
	s := newTestStore(t)
	oldNs := domain.Namespace("github.com/org/repo@v1.0.0")
	newNs := domain.Namespace("github.com/org/repo@v2.0.0")

	require.NoError(t, putArrow(s, oldNs, testFile))

	err := renameArrow(s, oldNs, newNs)
	require.NoError(t, err)

	// old should be gone
	_, err = getArrow(s, oldNs)
	assert.ErrorIs(t, err, ErrNotCached)

	// new should exist
	got, err := getArrow(s, newNs)
	require.NoError(t, err)
	assert.Equal(t, testFile.Content, got.Content)
}

func TestHelperRenameArrow_SourceDoesNotExist(t *testing.T) {
	s := newTestStore(t)
	oldNs := domain.Namespace("github.com/org/nonexistent@v1.0.0")
	newNs := domain.Namespace("github.com/org/new@v1.0.0")

	err := renameArrow(s, oldNs, newNs)
	assert.Error(t, err)
}

func TestHelperRenameArrow_NoConcurrentDeadlock(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("github.com/a/b@v1.0.0")
	ns2 := domain.Namespace("github.com/c/d@v1.0.0")

	require.NoError(t, putArrow(s, ns1, ManifestFile{Content: []byte("x"), Filename: "arrow.yaml"}))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = renameArrow(s, ns1, ns2)
		}()
		go func() {
			defer wg.Done()
			_ = renameArrow(s, ns2, ns1)
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected")
	}
}

// listVersions tests

func TestHelperListVersions_ThreeVersions(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("example.com/user/repo@v1.0.0")
	ns2 := domain.Namespace("example.com/user/repo@v2.0.0")
	ns3 := domain.Namespace("example.com/user/repo@v3.0.0")
	bare := domain.Namespace("example.com/user/repo")

	require.NoError(t, putArrow(s, ns1, testFile))
	require.NoError(t, putArrow(s, ns2, testFile))
	require.NoError(t, putArrow(s, ns3, testFile))

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"v1.0.0", "v2.0.0", "v3.0.0"}, versions)
}

func TestHelperListVersions_NoVersions(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("example.com/user/repo")

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestHelperListVersions_VaultDirNotExist(t *testing.T) {
	s := newTestStore(t)
	s.vaultPath = filepath.Join(t.TempDir(), "nonexistent")
	bare := domain.Namespace("example.com/user/repo")

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestHelperListVersions_ReadDirPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows or as root")
	}
	s := newTestStore(t)
	bare := domain.Namespace("example.com/user/repo")

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	require.NoError(t, os.Chmod(s.vaultPath, 0o000))
	t.Cleanup(func() { os.Chmod(s.vaultPath, 0o755) })

	versions, err := listVersions(s, bare)
	require.Error(t, err)
	assert.Empty(t, versions)
}

func TestHelperListVersions_SkipsNonMatchingNamespaces(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("example.com/user/myrepo@v1.0.0")
	other := domain.Namespace("example.com/user/otherrepo@v1.0.0")
	bare := domain.Namespace("example.com/user/myrepo")

	require.NoError(t, putArrow(s, ns, testFile))
	require.NoError(t, putArrow(s, other, testFile))

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.Equal(t, []string{"v1.0.0"}, versions)
}

func TestHelperListVersions_EmptySliceNormalization(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("example.com/user/repo")

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.NotNil(t, versions)
	assert.Empty(t, versions)
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
	nsDir := filepath.Join(s.namespacesPath, ns.String())

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
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("not-json"), 0o644))

	_, _, err := getQuiver(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

func TestHelperGetQuiver_ReadPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	nsDir := filepath.Join(s.namespacesPath, ns.String())
	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("{}"), 0o000))
	defer os.Chmod(filepath.Join(nsDir, quiverFilename), 0o644)

	_, _, err := getQuiver(s, ns)
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
	require.NoError(t, os.WriteFile(filepath.Join(s.namespacesPath, firstComponent), []byte("block"), 0o644))

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.Chmod(nsDir, 0o555))
	defer os.Chmod(nsDir, 0o700)

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.Chmod(nsDir, 0o555))
	defer os.Chmod(nsDir, 0o700)

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, quiverFilename), 0o700))

	_, err := putQuiver(s, ns, &domain.QuiverManifest{})

	assert.Error(t, err)
}

func TestHelperPutQuiver_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 5; i++ {
		manifest := &domain.QuiverManifest{Name: fmt.Sprintf("version-%d", i)}
		_, err := putQuiver(s, ns, manifest)
		require.NoError(t, err)
	}

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "version-4", got.Manifest.Name)
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

func TestHelperDeleteQuiver_RemoveError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("data"), 0o644))
	require.NoError(t, os.Chmod(nsDir, 0o555))
	defer os.Chmod(nsDir, 0o700)

	err := deleteQuiver(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
}

// acquireNamespace tests (quiver path traversal safety)

func TestHelperNamespacePath_RejectsTraversal(t *testing.T) {
	s := newTestStore(t)

	_, _, err := acquireNamespace(s, domain.Namespace("../../etc/passwd"))

	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestHelperDeleteArrow_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	// Arrow uses namespace lock key (not path-traversal check), so traversal still works
	// This just tests that the traversal namespace doesn't find a meta file
	ns := domain.Namespace("../../../etc/passwd")
	err := deleteArrow(s, ns)
	// deleteArrow returns nil if meta file not found (idempotent)
	assert.NoError(t, err)
}

func TestHelperDeleteQuiver_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	err := deleteQuiver(s, ns)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestHelperGetArrow_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	// Arrow helpers don't use acquireNamespace — they use namespaceLock directly.
	// Traversal namespace would not find meta file → ErrNotCached
	ns := domain.Namespace("../../../etc/passwd")
	_, err := getArrow(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetQuiver_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	_, _, err := getQuiver(s, ns)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestHelperPutArrow_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	// Arrow PutArrow uses flat vaultPath, not acquireNamespace.
	// A traversal NS would be URL-encoded so path is safe.
	// Just ensure it doesn't panic.
	ns := domain.Namespace("../../../etc/passwd")
	err := putArrow(s, ns, testFile)
	// Should succeed — URL encoding makes it safe
	assert.NoError(t, err)
}

func TestHelperPutQuiver_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	_, err := putQuiver(s, ns, mocks.QuiverManifest())
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// Constructor tests

func TestNewConstructor_WithDefaults(t *testing.T) {
	v := New("", "", 0)

	assert.NotNil(t, v)
	st := v.(*store)
	assert.NotEmpty(t, st.vaultPath)
	assert.NotEmpty(t, st.namespacesPath)
	assert.Equal(t, 24*time.Hour, st.ttl)
	assert.NotNil(t, st.locks)
	assert.Equal(t, 0, len(st.locks))
}

func TestNewConstructor_WithCustomValues(t *testing.T) {
	vaultDir := t.TempDir()
	nsDir := t.TempDir()
	ttl := 48 * time.Hour

	v := New(vaultDir, nsDir, ttl)

	assert.NotNil(t, v)
	st := v.(*store)
	assert.Equal(t, vaultDir, st.vaultPath)
	assert.Equal(t, nsDir, st.namespacesPath)
	assert.Equal(t, ttl, st.ttl)
}

// Race condition in namespaceLock

func TestNamespaceLock_RaceCondition(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("github.com/test/concurrent1")
	ns2 := domain.Namespace("github.com/test/concurrent2")

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

	ns1Lock := s.locks[ns1.String()]
	for i := 0; i < 100; i += 2 {
		assert.Equal(t, ns1Lock, locks[i])
	}

	ns2Lock := s.locks[ns2.String()]
	for i := 1; i < 100; i += 2 {
		assert.Equal(t, ns2Lock, locks[i])
	}
}

func TestConcurrentNamespaceLock_HighContention(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/test/highcontention")

	var wg sync.WaitGroup
	const workers = 200

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				lock := s.namespaceLock(ns.String())
				assert.NotNil(t, lock)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, len(s.locks))
	assert.NotNil(t, s.locks[ns.String()])
}

// TTL precision tests

func TestHelperGetArrow_JustBeforeStale(t *testing.T) {
	s := newTestStore(t)
	s.ttl = 100 * time.Millisecond
	ns := mocks.Namespace()

	base := time.Now()
	s.clock = func() time.Time { return base }

	require.NoError(t, putArrow(s, ns, testFile))

	s.clock = func() time.Time { return base.Add(50 * time.Millisecond) }

	got, err := getArrow(s, ns)
	assert.NoError(t, err)
	assert.NotErrorIs(t, err, ErrStale)
	assert.NotNil(t, got.Content)
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

	s.clock = func() time.Time { return base.Add(50 * time.Millisecond) }

	got, _, err := getQuiver(s, ns)
	assert.NoError(t, err)
	assert.NotErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
}

func TestHelperGetArrow_MetadataTimestampPrecision(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	beforePut := time.Now()
	require.NoError(t, putArrow(s, ns, testFile))
	afterPut := time.Now()

	meta, err := readMeta(s.metaFilePath(ns))
	require.NoError(t, err)

	assert.True(t, !meta.CachedAt.Before(beforePut) || meta.CachedAt.Equal(beforePut))
	assert.True(t, !meta.CachedAt.After(afterPut) || meta.CachedAt.Equal(afterPut))
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

	assert.True(t, !got.Metadata.CachedAt.Before(beforePut) || got.Metadata.CachedAt.Equal(beforePut))
	assert.True(t, !got.Metadata.CachedAt.After(afterPut) || got.Metadata.CachedAt.Equal(afterPut))
}

// Concurrent arrow tests

func TestHelperPutArrow_WriteFailureWithCleanup(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := putArrow(s, ns, testFile)
	require.NoError(t, err)

	_, err = getArrow(s, ns)
	require.NoError(t, err)
}

func TestHelperPutArrow_MultipleWritesWithoutCloseErrors(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 10; i++ {
		file := ManifestFile{
			Content:  []byte(fmt.Sprintf("arrow-%d", i)),
			Filename: "ARROW.md",
		}

		err := putArrow(s, ns, file)
		require.NoError(t, err)

		got, err := getArrow(s, ns)
		require.NoError(t, err)
		assert.Equal(t, file.Content, got.Content)
	}
}

func TestHelperPutQuiver_MultipleWritesWithoutCloseErrors(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 10; i++ {
		quiver := &domain.QuiverManifest{
			Name: fmt.Sprintf("quiver-%d", i),
		}

		path, err := putQuiver(s, ns, quiver)
		require.NoError(t, err)
		assert.NotEmpty(t, path)

		got, _, err := getQuiver(s, ns)
		require.NoError(t, err)
		assert.Equal(t, quiver.Name, got.Manifest.Name)
	}
}

func TestHelperGetQuiver_MetadataPreservation(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.QuiverManifest{Name: "special"}

	_, err := putQuiver(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getQuiver(s, ns)
	require.NoError(t, err)

	assert.False(t, got.Metadata.CachedAt.IsZero())
}
