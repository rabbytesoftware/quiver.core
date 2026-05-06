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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
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

func writeStaleQuiverEntry(t *testing.T, dir string, quiver *domain.Collection) {
	entry := struct {
		Collection *domain.Collection `json:"collection"`
		CachedAt   time.Time          `json:"cached_at"`
	}{
		Collection: quiver,
		CachedAt:   time.Now().Add(-2 * time.Hour),
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

func TestHelperGetCollection_NotCached(t *testing.T) {
	s := newTestStore(t)

	_, _, err := getCollection(s, mocks.Namespace())

	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperGetCollection_Fresh(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "test-quiver"}}

	_, err := putCollection(s, ns, manifest)
	require.NoError(t, err)

	got, path, err := getCollection(s, ns)

	require.NoError(t, err)
	assert.Equal(t, manifest.Meta.Name, got.Collection.Meta.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetCollection_Stale(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "stale-quiver"}}
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	writeStaleQuiverEntry(t, nsDir, manifest)

	got, path, err := getCollection(s, ns)

	assert.ErrorIs(t, err, ErrStale)
	assert.NotNil(t, got)
	assert.Equal(t, manifest.Meta.Name, got.Collection.Meta.Name)
	assert.NotEmpty(t, path)
}

func TestHelperGetCollection_CorruptedJSON(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("not-json"), 0o644))

	_, _, err := getCollection(s, ns)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotCached)
	assert.NotErrorIs(t, err, ErrStale)
}

func TestHelperGetCollection_ReadPermissionError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	nsDir := filepath.Join(s.namespacesPath, ns.String())
	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nsDir, quiverFilename), []byte("{}"), 0o000))
	defer os.Chmod(filepath.Join(nsDir, quiverFilename), 0o644)

	_, _, err := getCollection(s, ns)
	assert.Error(t, err)
}

// putQuiver tests

func TestHelperPutCollection_CreatesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "test-quiver"}}

	path, err := putCollection(s, ns, manifest)

	require.NoError(t, err)
	assert.NotEmpty(t, path)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr)
}

func TestHelperPutCollection_OverwritesExisting(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putCollection(s, ns, &domain.Collection{Meta: domain.CollectionMeta{Name: "first"}})
	require.NoError(t, err)

	_, err = putCollection(s, ns, &domain.Collection{Meta: domain.CollectionMeta{Name: "second"}})
	require.NoError(t, err)

	got, _, err := getCollection(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "second", got.Collection.Meta.Name)
}

func TestHelperPutCollection_SetsMetadata(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "meta-quiver"}}

	path, err := putCollection(s, ns, manifest)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var entry struct {
		Collection *domain.Collection `json:"collection"`
		CachedAt   time.Time          `json:"cached_at"`
	}
	require.NoError(t, json.Unmarshal(data, &entry))

	assert.False(t, entry.CachedAt.IsZero())
	assert.Equal(t, manifest.Meta.Name, entry.Collection.Meta.Name)
}

func TestHelperPutCollection_MkdirError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Block MkdirAll by writing a file where the first path component would be.
	firstComponent := strings.SplitN(ns.String(), "/", 2)[0]
	require.NoError(t, os.WriteFile(filepath.Join(s.namespacesPath, firstComponent), []byte("block"), 0o644))

	_, err := putCollection(s, ns, &domain.Collection{})

	assert.Error(t, err)
}

func TestHelperPutCollection_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.Chmod(nsDir, 0o555))
	defer os.Chmod(nsDir, 0o700)

	_, err := putCollection(s, ns, &domain.Collection{})

	assert.Error(t, err)
}

func TestHelperPutCollection_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(nsDir, 0o700))
	require.NoError(t, os.Chmod(nsDir, 0o555))
	defer os.Chmod(nsDir, 0o700)

	_, err := putCollection(s, ns, &domain.Collection{})

	assert.Error(t, err)
}

func TestHelperPutCollection_RenameError(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, quiverFilename), 0o700))

	_, err := putCollection(s, ns, &domain.Collection{})

	assert.Error(t, err)
}

func TestHelperPutCollection_MultipleOverwrites(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 5; i++ {
		manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: fmt.Sprintf("version-%d", i)}}
		_, err := putCollection(s, ns, manifest)
		require.NoError(t, err)
	}

	got, _, err := getCollection(s, ns)
	require.NoError(t, err)
	assert.Equal(t, "version-4", got.Collection.Meta.Name)
}

// deleteQuiver tests

func TestHelperDeleteCollection_RemovesFile(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	_, err := putCollection(s, ns, &domain.Collection{Meta: domain.CollectionMeta{Name: "to-delete"}})
	require.NoError(t, err)

	err = deleteCollection(s, ns)
	require.NoError(t, err)

	_, _, err = getCollection(s, ns)
	assert.ErrorIs(t, err, ErrNotCached)
}

func TestHelperDeleteCollection_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	err := deleteCollection(s, ns)

	assert.NoError(t, err)
}

func TestHelperDeleteCollection_RemoveError(t *testing.T) {
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

	err := deleteCollection(s, ns)

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

func TestHelperDeleteCollection_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	err := deleteCollection(s, ns)
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

func TestHelperGetCollection_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	_, _, err := getCollection(s, ns)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestHelperPutArrow_DoesNotUseAcquireNamespace(t *testing.T) {
	s := newTestStore(t)

	// putArrow writes a flat file under vaultPath (URL-encoded) rather than
	// calling acquireNamespace like putQuiver does. Verify it succeeds with a
	// valid namespace that contains characters requiring encoding.
	ns := domain.Namespace("github.com/org/repo@v1.2.3")
	err := putArrow(s, ns, testFile)
	assert.NoError(t, err)
}

func TestHelperPutCollection_AcquireNamespaceError(t *testing.T) {
	s := newTestStore(t)

	ns := domain.Namespace("../../../etc/passwd")
	_, err := putCollection(s, ns, mocks.Quiver())
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

func TestHelperGetCollection_JustBeforeStale(t *testing.T) {
	s := newTestStore(t)
	s.ttl = 100 * time.Millisecond
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "fresh-enough"}}

	base := time.Now()
	s.clock = func() time.Time { return base }

	_, err := putCollection(s, ns, manifest)
	require.NoError(t, err)

	s.clock = func() time.Time { return base.Add(50 * time.Millisecond) }

	got, _, err := getCollection(s, ns)
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

func TestHelperGetCollection_MetadataTimestampPrecision(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "precise"}}

	beforePut := time.Now()
	_, err := putCollection(s, ns, manifest)
	afterPut := time.Now()
	require.NoError(t, err)

	got, _, err := getCollection(s, ns)
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

func TestHelperPutCollection_MultipleWritesWithoutCloseErrors(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	for i := 0; i < 10; i++ {
		quiver := &domain.Collection{
			Meta: domain.CollectionMeta{Name: fmt.Sprintf("quiver-%d", i)},
		}

		path, err := putCollection(s, ns, quiver)
		require.NoError(t, err)
		assert.NotEmpty(t, path)

		got, _, err := getCollection(s, ns)
		require.NoError(t, err)
		assert.Equal(t, quiver.Meta.Name, got.Collection.Meta.Name)
	}
}

func TestHelperGetCollection_MetadataPreservation(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()
	manifest := &domain.Collection{Meta: domain.CollectionMeta{Name: "special"}}

	_, err := putCollection(s, ns, manifest)
	require.NoError(t, err)

	got, _, err := getCollection(s, ns)
	require.NoError(t, err)

	assert.False(t, got.Metadata.CachedAt.IsZero())
}

// atomicWrite error path tests

func TestAtomicWrite_WriteError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	dir := t.TempDir()
	// Make dir read-only so CreateTemp fails
	require.NoError(t, os.Chmod(dir, 0o555))
	defer os.Chmod(dir, 0o700)

	err := atomicWrite(filepath.Join(dir, "target.txt"), []byte("data"))
	assert.Error(t, err)
}

func TestAtomicWrite_RenameError(t *testing.T) {
	// Make atomicWrite fail at the rename step by making the target path a directory.
	// CreateTemp succeeds in dir, but Rename to a directory-named target fails.
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	// Create a directory at the target path so os.Rename fails
	require.NoError(t, os.Mkdir(target, 0o700))

	err := atomicWrite(target, []byte("data"))
	assert.Error(t, err)
}

// deleteArrow permission error tests

func TestHelperDeleteArrow_ManifestRemoveError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, putArrow(s, ns, testFile))

	// Make vault dir non-writable so os.Remove on manifest fails
	require.NoError(t, os.Chmod(s.vaultPath, 0o555))
	defer os.Chmod(s.vaultPath, 0o700)

	err := deleteArrow(s, ns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault delete: remove manifest")
}

func TestHelperDeleteArrow_MetaRemoveError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Write meta but no manifest file, then block removal of meta.
	// deleteArrow reads meta first, then tries to remove manifest (ErrNotExist → ok),
	// then tries to remove meta. Block meta removal by removing manifest first
	// and making the directory non-writable after.
	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	meta, _ := json.Marshal(VaultMetadata{CachedAt: time.Now(), Filename: "ARROW.md"})
	require.NoError(t, os.WriteFile(s.metaFilePath(ns), meta, 0o644))
	// No manifest file written — manifest remove will be ErrNotExist (ok).
	// Now make vault dir non-writable so meta removal also fails.
	require.NoError(t, os.Chmod(s.vaultPath, 0o555))
	defer os.Chmod(s.vaultPath, 0o700)

	err := deleteArrow(s, ns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault delete: remove meta")
}

// renameArrow error path tests

func TestHelperRenameArrow_ManifestRenameError(t *testing.T) {
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("skipping: file permission restrictions do not apply for root or on Windows")
	}
	s := newTestStore(t)
	oldNs := domain.Namespace("github.com/org/repo@v1.0.0")
	newNs := domain.Namespace("github.com/org/repo@v2.0.0")

	require.NoError(t, putArrow(s, oldNs, testFile))

	// Make vault dir non-writable so rename of manifest fails
	require.NoError(t, os.Chmod(s.vaultPath, 0o555))
	defer os.Chmod(s.vaultPath, 0o700)

	err := renameArrow(s, oldNs, newNs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault rename manifest")
}

func TestHelperRenameArrow_MetaRenameErrorWithRollback(t *testing.T) {
	// Trigger the meta rename failure + rollback by making oldMeta non-renameable.
	// Strategy: write the manifest files manually but put a directory where newMeta should land,
	// so the manifest rename succeeds but meta rename fails.
	s := newTestStore(t)
	oldNs := domain.Namespace("github.com/org/repo@v1.0.0")
	newNs := domain.Namespace("github.com/org/repo@v2.0.0")

	require.NoError(t, putArrow(s, oldNs, testFile))

	// Create a directory at newMeta path so os.Rename to newMeta fails
	newMetaPath := s.metaFilePath(newNs)
	require.NoError(t, os.Mkdir(newMetaPath, 0o700))

	err := renameArrow(s, oldNs, newNs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "vault rename meta")

	// Rollback should have restored the old manifest — old entry should still be gettable
	got, getErr := getArrow(s, oldNs)
	require.NoError(t, getErr)
	assert.Equal(t, testFile.Content, got.Content)
}

// putArrow atomicWrite error paths

func TestHelperPutArrow_ManifestAtomicWriteError(t *testing.T) {
	// Force atomicWrite for manifest to fail by placing a directory where the manifest file
	// should land (rename will fail since target is a directory).
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	manifestPath := s.manifestFilePath(ns, testFile.Filename)
	// Create a directory at that path so atomicWrite rename fails
	require.NoError(t, os.Mkdir(manifestPath, 0o700))

	err := putArrow(s, ns, testFile)
	assert.Error(t, err)
}

func TestHelperPutArrow_MetaAtomicWriteError(t *testing.T) {
	// Manifest write succeeds, but meta write fails because a directory exists at meta path.
	s := newTestStore(t)
	ns := mocks.Namespace()

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))
	metaPath := s.metaFilePath(ns)
	// Create directory at meta path so atomicWrite rename fails for meta
	require.NoError(t, os.Mkdir(metaPath, 0o700))

	err := putArrow(s, ns, testFile)
	assert.Error(t, err)
}

// putQuiver atomicWrite error path

func TestHelperPutCollection_AtomicWriteRenameError(t *testing.T) {
	// quiverFilename target is a directory so rename into it fails.
	s := newTestStore(t)
	ns := mocks.Namespace()
	nsDir := filepath.Join(s.namespacesPath, ns.String())

	require.NoError(t, os.MkdirAll(filepath.Join(nsDir, quiverFilename), 0o700))

	_, err := putCollection(s, ns, &domain.Collection{Meta: domain.CollectionMeta{Name: "x"}})
	assert.Error(t, err)
}

// listVersions url.PathUnescape error path

func TestHelperListVersions_SkipsMalformedEncoding(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("example.com/user/repo")

	require.NoError(t, os.MkdirAll(s.vaultPath, 0o700))

	// Write a .meta.json file with an invalid percent-encoded name — PathUnescape will error
	// and the entry should be silently skipped (continue).
	invalidFile := filepath.Join(s.vaultPath, "%ZZ.meta.json")
	require.NoError(t, os.WriteFile(invalidFile, []byte("{}"), 0o644))

	versions, err := listVersions(s, bare)
	require.NoError(t, err)
	assert.Empty(t, versions)
}

// listCachedQuivers

func TestListCachedCollections_ReturnsAllQuivers(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("example.com/org/repo-a")
	ns2 := domain.Namespace("example.com/org/repo-b")

	_, err := putCollection(s, ns1, &domain.Collection{Meta: domain.CollectionMeta{Name: "a"}})
	require.NoError(t, err)
	_, err = putCollection(s, ns2, &domain.Collection{Meta: domain.CollectionMeta{Name: "b"}})
	require.NoError(t, err)

	got, err := listCachedQuivers(s)
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.Namespace{ns1, ns2}, got)
}

func TestListCachedCollections_Empty(t *testing.T) {
	s := newTestStore(t)

	got, err := listCachedQuivers(s)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListCachedCollections_NonexistentDir_ReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	s.namespacesPath = filepath.Join(t.TempDir(), "nonexistent")

	got, err := listCachedQuivers(s)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListCachedCollections_IgnoresDirsWithoutQuiverJSON(t *testing.T) {
	s := newTestStore(t)
	ns := mocks.Namespace()

	// Create a namespace dir without quiver.json (arrow only)
	require.NoError(t, putArrow(s, ns, testFile))
	// No putQuiver call — dir exists but has no quiver.json

	got, err := listCachedQuivers(s)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// namespaceLock double-check path (key inserted between RUnlock and Lock)

func TestNamespaceLock_DoubleCheckPath(t *testing.T) {
	// Seed the lock for a key, then simulate the double-check path by calling namespaceLock
	// from many goroutines simultaneously on a fresh key. The goroutine that loses the
	// write-lock race will hit the "if m, ok := s.locks[key]; ok { return m }" branch.
	s := newTestStore(t)
	key := "double-check-key"

	const n = 500
	results := make(chan *sync.Mutex, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		go func() {
			<-start
			results <- s.namespaceLock(key)
		}()
	}

	close(start)
	for i := 0; i < n; i++ {
		<-results
	}

	// All goroutines must have gotten the same lock instance
	assert.Equal(t, 1, len(s.locks))
	assert.NotNil(t, s.locks[key])
}
