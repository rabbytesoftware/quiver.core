//go:build integration

package kit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// FixtureRepos is a concurrency-safe map of fixture key (e.g. "quiver-test/tool-a")
// to its in-memory storer. All access is synchronized so test threads and resolver
// goroutines can safely read/write the same instance.
type FixtureRepos struct {
	mu    sync.RWMutex
	store map[string]*memory.Storage
}

func newFixtureRepos() *FixtureRepos {
	return &FixtureRepos{store: make(map[string]*memory.Storage)}
}

func (f *FixtureRepos) Get(key string) (*memory.Storage, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	s, ok := f.store[key]
	return s, ok
}

func (f *FixtureRepos) Set(key string, s *memory.Storage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = s
}

func (f *FixtureRepos) Delete(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.store, key)
}

var versionDirRe = regexp.MustCompile(`^v\d+$`)

// testdataArrowsDir returns the path to testdata/arrows/ relative to any suite package.
// All suite packages live one level below tests/integration/.
func testdataArrowsDir() string {
	return filepath.Join("..", "testdata", "arrows")
}

// BuildFixtureRepos walks testdata/arrows/ and builds one in-memory git repo per fixture.
func BuildFixtureRepos(t *testing.T) *FixtureRepos {
	t.Helper()

	root := testdataArrowsDir()
	repos := newFixtureRepos()

	err := walkFixtures(root, func(relDir string, versionedFiles map[string]string) {
		key := dirToKey(relDir)

		storer := memory.NewStorage()
		fs := memfs.New()
		repo, err := gogit.Init(storer, fs)
		if err != nil {
			t.Fatalf("git init for %s: %v", key, err)
		}

		wt, err := repo.Worktree()
		if err != nil {
			t.Fatalf("worktree for %s: %v", key, err)
		}

		if len(versionedFiles) > 0 {
			for _, tag := range sortedVersionTags(versionedFiles) {
				content := versionedFiles[tag]
				commitFile(t, wt, "arrow.yaml", []byte(content))
				hash, err := wt.Commit(
					fmt.Sprintf("version %s", tag),
					&gogit.CommitOptions{
						Author:            testAuthor(),
						AllowEmptyCommits: false,
					},
				)
				if err != nil {
					t.Fatalf("commit %s for %s: %v", tag, key, err)
				}
				createTag(t, repo, tag, hash)
			}
			repos.Set(key, storer)
			return
		}

		filename, content := readManifestFile(t, root, relDir, key)

		commitFile(t, wt, filename, content)
		hash, err := wt.Commit(
			"init",
			&gogit.CommitOptions{
				Author:            testAuthor(),
				AllowEmptyCommits: false,
			},
		)
		if err != nil {
			t.Fatalf("commit for %s: %v", key, err)
		}
		createTag(t, repo, "v1", hash)
		repos.Set(key, storer)
	})

	if err != nil {
		t.Fatalf("walkFixtures: %v", err)
	}

	return repos
}

// BuildUpgradeRepo creates an in-memory repo with only v1 committed and tagged.
// Use AddV2ToRepo to inject v2 mid-test for upgrade path tests.
func BuildUpgradeRepo(t *testing.T, v1Content []byte) *memory.Storage {
	t.Helper()

	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := gogit.Init(storer, fs)
	if err != nil {
		t.Fatalf("BuildUpgradeRepo: git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("BuildUpgradeRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", v1Content)
	hash, err := wt.Commit("v1", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("BuildUpgradeRepo: commit v1: %v", err)
	}

	createTag(t, repo, "v1", hash)
	return storer
}

// AddV2ToRepo adds a v2 commit and tag to an existing in-memory storer.
func AddV2ToRepo(t *testing.T, storer *memory.Storage, v2Content []byte) {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("AddV2ToRepo: open repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("AddV2ToRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", v2Content)
	hash, err := wt.Commit("v2", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("AddV2ToRepo: commit v2: %v", err)
	}

	createTag(t, repo, "v2", hash)
}

// testResolver implements resolver.Resolver and resolvers.ConstraintResolver
// using in-memory fixture repos. The mutex serializes repo I/O because
// go-git's memory.Storage is not safe for concurrent use. Map access is
// handled by FixtureRepos's own lock.
type testResolver struct {
	mu    sync.Mutex
	repos *FixtureRepos
}

func newTestResolver(repos *FixtureRepos) *testResolver {
	return &testResolver{repos: repos}
}

// fixtureKey strips the "quiver.test/" prefix to get the fixture map key.
func fixtureKey(ns domain.Namespace) string {
	bare := string(ns.BareNamespace())
	return strings.TrimPrefix(bare, "quiver.test/")
}

func (r *testResolver) ResolveArrow(ctx context.Context, ns domain.Namespace) ([]byte, string, error) {
	key := fixtureKey(ns)
	storer, ok := r.repos.Get(key)
	if !ok {
		return nil, "", fmt.Errorf("fixture repo not found: %s (key=%s)", ns, key)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if data, err := readFromRepo(storer, ns.Ref(), "ARROW.md"); err == nil {
		return data, "ARROW.md", nil
	}
	data, err := readFromRepo(storer, ns.Ref(), "arrow.yaml")
	if err != nil {
		return nil, "", err
	}
	return data, "arrow.yaml", nil
}

func (r *testResolver) ResolveQuiver(_ context.Context, _ domain.Namespace) ([]byte, error) {
	return nil, fmt.Errorf("quiver manifests not supported in integration tests")
}

func (r *testResolver) Resolve(_ context.Context, ns domain.Namespace, pattern string) (string, error) {
	key := fixtureKey(ns)
	storer, ok := r.repos.Get(key)
	if !ok {
		return "", fmt.Errorf("fixture repo not found for constraint: %s", ns)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return resolveConstraintFromTags(storer, pattern)
}

// --- internal git helpers ---

func walkFixtures(root string, fn func(relDir string, versionedFiles map[string]string)) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", root, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		subPath := filepath.Join(root, name)

		if isVersionedParent(subPath) {
			vfiles, err := collectVersionedFiles(subPath)
			if err != nil {
				return err
			}
			fn(name, vfiles)
			continue
		}

		if hasManifestFile(subPath) {
			fn(name, nil)
			continue
		}

		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			return fmt.Errorf("readdir %s: %w", subPath, err)
		}
		for _, se := range subEntries {
			if !se.IsDir() {
				continue
			}
			leafPath := filepath.Join(subPath, se.Name())
			if hasManifestFile(leafPath) {
				fn(filepath.Join(name, se.Name()), nil)
			}
		}
	}
	return nil
}

func isVersionedParent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return false
	}
	hasDir := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		hasDir = true
		if !versionDirRe.MatchString(e.Name()) {
			return false
		}
	}
	return hasDir
}

func collectVersionedFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tag := e.Name()
		yamlPath := filepath.Join(dir, tag, "arrow.yaml")
		data, err := os.ReadFile(yamlPath) // #nosec G304 -- path is under testdata/, controlled by test fixtures only
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", yamlPath, err)
		}
		out[tag] = string(data)
	}
	return out, nil
}

func sortedVersionTags(vfiles map[string]string) []string {
	tags := make([]string, 0, len(vfiles))
	for t := range vfiles {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

func dirToKey(relDir string) string {
	key := filepath.ToSlash(relDir)
	parts := strings.SplitN(key, "/", 2)
	if len(parts) == 1 {
		return "quiver-test/" + key
	}
	return key
}

func commitFile(t *testing.T, wt *gogit.Worktree, filename string, content []byte) {
	t.Helper()
	f, err := wt.Filesystem.Create(filename)
	if err != nil {
		t.Fatalf("create %s in worktree: %v", filename, err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		t.Fatalf("write %s in worktree: %v", filename, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s in worktree: %v", filename, err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("stage %s: %v", filename, err)
	}
}

// hasManifestFile returns true if dir contains ARROW.md or arrow.yaml.
func hasManifestFile(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "ARROW.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "arrow.yaml")); err == nil {
		return true
	}
	return false
}

// readManifestFile reads the manifest file from root/relDir, preferring ARROW.md over arrow.yaml.
// Returns the filename and content.
func readManifestFile(t *testing.T, root, relDir, key string) (string, []byte) {
	t.Helper()
	mdPath := filepath.Join(root, relDir, "ARROW.md")
	if content, err := os.ReadFile(mdPath); err == nil { // #nosec G304 -- path is under testdata/, controlled by test fixtures only
		return "ARROW.md", content
	}
	yamlPath := filepath.Join(root, relDir, "arrow.yaml")
	content, err := os.ReadFile(yamlPath) // #nosec G304 -- path is under testdata/, controlled by test fixtures only
	if err != nil {
		t.Fatalf("read manifest for %s: %v", key, err)
	}
	return "arrow.yaml", content
}

func createTag(t *testing.T, repo *gogit.Repository, tag string, hash plumbing.Hash) {
	t.Helper()
	if err := repo.Storer.SetReference(
		plumbing.NewHashReference(plumbing.NewTagReferenceName(tag), hash),
	); err != nil {
		t.Fatalf("create tag %s: %v", tag, err)
	}
}

func testAuthor() *object.Signature {
	return &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
}

func readFromRepo(storer *memory.Storage, ref string, filename string) ([]byte, error) {
	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	var commitHash plumbing.Hash
	if ref == "" {
		head, err := repo.Head()
		if err != nil {
			return nil, fmt.Errorf("resolve HEAD: %w", err)
		}
		commitHash = head.Hash()
	} else {
		tagRef, err := repo.Storer.Reference(plumbing.NewTagReferenceName(ref))
		if err == nil {
			commitHash = tagRef.Hash()
		} else {
			branchRef, err := repo.Storer.Reference(plumbing.NewBranchReferenceName(ref))
			if err != nil {
				return nil, fmt.Errorf("resolve ref %q: %w", ref, err)
			}
			commitHash = branchRef.Hash()
		}
	}
	commit, err := repo.CommitObject(commitHash)
	if err != nil {
		tagObj, terr := repo.TagObject(commitHash)
		if terr != nil {
			return nil, fmt.Errorf("commit object %s: %w", commitHash, err)
		}
		commit, err = repo.CommitObject(tagObj.Target)
		if err != nil {
			return nil, fmt.Errorf("commit from tag target %s: %w", tagObj.Target, err)
		}
	}
	file, err := commit.File(filename)
	if err != nil {
		return nil, fmt.Errorf("file %s at %s: %w", filename, commitHash, err)
	}
	reader, err := file.Reader()
	if err != nil {
		return nil, fmt.Errorf("reader for %s: %w", filename, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}
	return data, nil
}

func resolveConstraintFromTags(storer *memory.Storage, pattern string) (string, error) {
	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		return "", fmt.Errorf("open repo: %w", err)
	}
	tagIter, err := repo.Tags()
	if err != nil {
		return "", fmt.Errorf("list tags: %w", err)
	}
	defer tagIter.Close()
	var matched []string
	err = tagIter.ForEach(func(ref *plumbing.Reference) error {
		tagName := ref.Name().Short()
		ok, merr := path.Match(pattern, tagName)
		if merr != nil {
			return fmt.Errorf("invalid pattern %q: %w", pattern, merr)
		}
		if ok {
			matched = append(matched, tagName)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("iterate tags: %w", err)
	}
	if len(matched) == 0 {
		return "", fmt.Errorf("constraint: no git tags match pattern %q", pattern)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matched)))
	return matched[0], nil
}
