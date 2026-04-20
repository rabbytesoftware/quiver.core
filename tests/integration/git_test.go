//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
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

// fixtureRepos maps a fixture key (e.g. "quiver-test/tool-a") to its in-memory storer.
type fixtureRepos map[string]*memory.Storage

var versionDirRe = regexp.MustCompile(`^v\d+$`)

func testdataArrowsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", "arrows")
}

// buildFixtureRepos walks testdata/arrows/ and builds one in-memory git repo per fixture.
func buildFixtureRepos(t *testing.T) fixtureRepos {
	t.Helper()

	root := testdataArrowsDir()
	repos := make(fixtureRepos)

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
			// Multi-version fixture: commit each version in order.
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
			repos[key] = storer
			return
		}

		// Single-version fixture.
		yamlPath := filepath.Join(root, relDir, "arrow.yaml")
		content, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("read arrow.yaml for %s: %v", key, err)
		}

		commitFile(t, wt, "arrow.yaml", content)
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
		repos[key] = storer
	})

	if err != nil {
		t.Fatalf("walkFixtures: %v", err)
	}

	return repos
}

// walkFixtures calls fn for each fixture found under root.
// relDir is relative to root.
// versionedFiles is non-nil only for multi-version fixtures; keys are tag names ("v1", "v2", ...).
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

		// Check if this directory is a multi-version fixture (contains only v\d+ subdirs).
		if isVersionedParent(subPath) {
			vfiles, err := collectVersionedFiles(subPath)
			if err != nil {
				return err
			}
			fn(name, vfiles)
			continue
		}

		// Check if arrow.yaml is directly in this directory.
		if _, err := os.Stat(filepath.Join(subPath, "arrow.yaml")); err == nil {
			fn(name, nil)
			continue
		}

		// Recurse one level for nested fixtures (dep-chain, dep-diamond, etc.).
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			return fmt.Errorf("readdir %s: %w", subPath, err)
		}

		for _, se := range subEntries {
			if !se.IsDir() {
				continue
			}
			subName := se.Name()
			leafPath := filepath.Join(subPath, subName)

			if _, err := os.Stat(filepath.Join(leafPath, "arrow.yaml")); err == nil {
				fn(filepath.Join(name, subName), nil)
			}
		}
	}

	return nil
}

// isVersionedParent returns true if dir contains only subdirectories matching v\d+.
func isVersionedParent(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return false
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue // skip files like .DS_Store
		}
		if !versionDirRe.MatchString(e.Name()) {
			return false
		}
	}
	return true
}

// collectVersionedFiles reads all v\d+ subdirs and returns map[tag]contents.
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
		data, err := os.ReadFile(yamlPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", yamlPath, err)
		}
		out[tag] = string(data)
	}
	return out, nil
}

// sortedVersionTags returns version tags ("v1", "v2", ...) in ascending order.
func sortedVersionTags(vfiles map[string]string) []string {
	tags := make([]string, 0, len(vfiles))
	for t := range vfiles {
		tags = append(tags, t)
	}
	// Sort ascending by numeric suffix so v1 is committed before v2, etc.
	for i := 0; i < len(tags)-1; i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[i] > tags[j] {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}
	return tags
}

// dirToKey converts a relative directory path (from testdata/arrows/) to a fixture key.
// Top-level dirs get a "quiver-test/" prefix; nested dirs are used as-is.
func dirToKey(relDir string) string {
	// filepath.Join normalizes separators; convert to forward slashes for the key.
	key := filepath.ToSlash(relDir)
	parts := strings.SplitN(key, "/", 2)
	if len(parts) == 1 {
		return "quiver-test/" + key
	}
	return key
}

// commitFile writes content to filename in the worktree and stages it.
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

// createTag creates a lightweight tag pointing at hash.
func createTag(t *testing.T, repo *gogit.Repository, tag string, hash plumbing.Hash) {
	t.Helper()

	if err := repo.Storer.SetReference(
		plumbing.NewHashReference(plumbing.NewTagReferenceName(tag), hash),
	); err != nil {
		t.Fatalf("create tag %s: %v", tag, err)
	}
}

// buildUpgradeRepo creates an in-memory repo with only v1 committed and tagged.
// v2 is added later via addV2ToRepo during upgrade path tests.
func buildUpgradeRepo(t *testing.T, v1Content []byte) *memory.Storage {
	t.Helper()

	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := gogit.Init(storer, fs)
	if err != nil {
		t.Fatalf("buildUpgradeRepo: git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("buildUpgradeRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", v1Content)
	hash, err := wt.Commit("v1", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("buildUpgradeRepo: commit v1: %v", err)
	}

	createTag(t, repo, "v1", hash)
	return storer
}

// addV2ToRepo adds a v2 commit and tag to an existing in-memory storer.
// It opens the repo with a fresh memfs (worktree) and writes v2Content fresh.
func addV2ToRepo(t *testing.T, storer *memory.Storage, v2Content []byte) {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("addV2ToRepo: open repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("addV2ToRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", v2Content)
	hash, err := wt.Commit("v2", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("addV2ToRepo: commit v2: %v", err)
	}

	createTag(t, repo, "v2", hash)
}

func testAuthor() *object.Signature {
	return &object.Signature{
		Name:  "test",
		Email: "test@test.com",
		When:  time.Now(),
	}
}

// testResolver implements resolver.Resolver and resolvers.ConstraintResolver
// by reading from in-memory fixture repos.
// mu serializes all repo accesses because go-git's memory.Storage is not
// safe for concurrent use (its lazy-init ConfigStorage races under -race).
type testResolver struct {
	mu    sync.Mutex
	repos fixtureRepos
}

// fixtureKey strips the "quiver.test/" prefix from a namespace's bare part.
// e.g. "quiver.test/quiver-test/tool-a@v1" → "quiver-test/tool-a"
func fixtureKey(ns domain.Namespace) string {
	bare := string(ns.BareNamespace())
	return strings.TrimPrefix(bare, "quiver.test/")
}

// ResolveArrow implements resolver.Resolver.
func (r *testResolver) ResolveArrow(ctx context.Context, ns domain.Namespace) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fixtureKey(ns)
	storer, ok := r.repos[key]
	if !ok {
		return nil, fmt.Errorf("fixture repo not found: %s (key=%s)", ns, key)
	}
	return readFromRepo(storer, ns.Ref(), "arrow.yaml")
}

// ResolveQuiver implements resolver.Resolver.
func (r *testResolver) ResolveQuiver(_ context.Context, _ domain.Namespace) ([]byte, error) {
	return nil, fmt.Errorf("quiver manifests not supported in integration tests")
}

// Resolve implements resolvers.ConstraintResolver.
func (r *testResolver) Resolve(_ context.Context, ns domain.Namespace, pattern string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fixtureKey(ns)
	storer, ok := r.repos[key]
	if !ok {
		return "", fmt.Errorf("fixture repo not found for constraint: %s", ns)
	}
	return resolveConstraintFromTags(storer, pattern)
}

// readFromRepo reads filename from the given storer at the given ref.
// If ref is empty, HEAD is used. Tags are resolved before branches.
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
		// Try lightweight tag reference first.
		tagRef, err := repo.Storer.Reference(plumbing.NewTagReferenceName(ref))
		if err == nil {
			commitHash = tagRef.Hash()
		} else {
			// Fall back to branch.
			branchRef, err := repo.Storer.Reference(plumbing.NewBranchReferenceName(ref))
			if err != nil {
				return nil, fmt.Errorf("resolve ref %q: %w", ref, err)
			}
			commitHash = branchRef.Hash()
		}
	}

	commit, err := repo.CommitObject(commitHash)
	if err != nil {
		// Ref may point to a tag object rather than a commit (annotated tag).
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

// resolveConstraintFromTags lists all tags in the repo, matches them against pattern,
// and returns the highest matching tag. Uses the same sort logic as the production resolver.
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

	sortTagsDesc(matched)
	return matched[0], nil
}

// sortTagsDesc sorts tag names in descending order (semver-aware, falls back to lexicographic).
func sortTagsDesc(tags []string) {
	sort.Slice(tags, func(i, j int) bool {
		return tags[i] > tags[j] // descending lexicographic (works for v1, v2, ... v9)
	})
}
