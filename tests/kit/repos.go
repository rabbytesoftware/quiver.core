//go:build integration

package kit

import (
	"context"
	"fmt"
	"io"
	"io/fs"
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
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// FixtureRepos is a concurrency-safe map of fixture key (e.g. "quiver-test/tool-a")
// to its in-memory storer. All access is synchronized so test threads and resolver
// goroutines can safely read/write the same instance.
type FixtureRepos struct {
	mu    sync.RWMutex
	store map[string]*memory.Storage

	// io serializes git I/O on every storer in the set. go-git's
	// memory.Storage is not safe for concurrent use, and a test rewrites a
	// repo (a moved tag, a new release) while the daemon may be reading it.
	io sync.Mutex
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

// Mutate runs fn while no resolver reads any repo of the set. Wrap every
// change a test makes to a repo a running daemon may read.
func (f *FixtureRepos) Mutate(fn func()) {
	f.io.Lock()
	defer f.io.Unlock()
	fn()
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
	return BuildTaggedRepo(t, "v1", v1Content)
}

// BuildTaggedRepo creates an in-memory repo with one commit of content as
// arrow.yaml, tagged tag — the shape a rolling tag such as "nightly" or a
// release such as "v1.2.0" starts from.
func BuildTaggedRepo(t *testing.T, tag string, content []byte) *memory.Storage {
	t.Helper()

	storer := memory.NewStorage()
	repo, err := gogit.Init(storer, memfs.New())
	if err != nil {
		t.Fatalf("BuildTaggedRepo: git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("BuildTaggedRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", content)
	hash, err := wt.Commit(tag, &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("BuildTaggedRepo: commit %s: %v", tag, err)
	}

	createTag(t, repo, tag, hash)
	return storer
}

// BuildBranchOnlyRepo builds a fixture repo with a single commit on its
// default branch and no tags at all — the shape a repository that has never
// cut a release takes, which forces refless resolution onto the
// default-branch fallback instead of a stable-tag match.
func BuildBranchOnlyRepo(t *testing.T, content []byte) *memory.Storage {
	t.Helper()

	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := gogit.Init(storer, fs)
	if err != nil {
		t.Fatalf("BuildBranchOnlyRepo: git init: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("BuildBranchOnlyRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", content)
	if _, err := wt.Commit("init", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	}); err != nil {
		t.Fatalf("BuildBranchOnlyRepo: commit: %v", err)
	}

	return storer
}

// AddCommitToRepo adds a new commit to the default branch without tagging
// it — simulates the branch moving forward on a repository that still has no
// releases, as opposed to AddV2ToRepo which simulates a new release.
func AddCommitToRepo(t *testing.T, storer *memory.Storage, content []byte) {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("AddCommitToRepo: open repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("AddCommitToRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", content)
	if _, err := wt.Commit("advance", &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	}); err != nil {
		t.Fatalf("AddCommitToRepo: commit: %v", err)
	}
}

// AddTaggedCommitToRepo adds a new commit on the default branch and tags it
// with an arbitrary, caller-chosen tag — unlike AddV2ToRepo, which always
// tags "v2". Use this when the test needs a specific stable-semver tag (e.g.
// "v1.1.0") that a stable channel or constraint selector must pick up.
func AddTaggedCommitToRepo(t *testing.T, storer *memory.Storage, tag string, content []byte) {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("AddTaggedCommitToRepo: open repo: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("AddTaggedCommitToRepo: worktree: %v", err)
	}

	commitFile(t, wt, "arrow.yaml", content)
	hash, err := wt.Commit(tag, &gogit.CommitOptions{
		Author:            testAuthor(),
		AllowEmptyCommits: false,
	})
	if err != nil {
		t.Fatalf("AddTaggedCommitToRepo: commit %s: %v", tag, err)
	}

	createTag(t, repo, tag, hash)
}

// AddV2ToRepo adds a v2 commit and tag to an existing in-memory storer and
// returns the commit's hash.
func AddV2ToRepo(t *testing.T, storer *memory.Storage, v2Content []byte) string {
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
	return hash.String()
}

// TagCommit returns the commit tag points at. Run it inside
// FixtureRepos.Mutate when a daemon may be reading the repo.
func TagCommit(t *testing.T, storer *memory.Storage, tag string) string {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("TagCommit: open repo: %v", err)
	}
	ref, err := storer.Reference(plumbing.NewTagReferenceName(tag))
	if err != nil {
		t.Fatalf("TagCommit: tag %s: %v", tag, err)
	}
	return peeledCommit(repo, ref.Hash()).String()
}

// moveMarkerFile is the file a moved tag's fresh commit adds, so the commit
// differs from the one the tag left while every manifest stays byte-identical.
const moveMarkerFile = ".quiver-test-moved"

// MoveTagToNewCommit force-moves tag the way a rolling release re-points its
// tag: refs/tags/<tag> is deleted and recreated at a fresh commit whose tree
// is the old one plus a marker file, so the manifest is unchanged and only
// the commit moved. It returns the new commit. The default branch and HEAD
// are left where they are. Run it inside FixtureRepos.Mutate when a daemon
// may be reading the repo.
func MoveTagToNewCommit(t *testing.T, storer *memory.Storage, tag string) string {
	t.Helper()

	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: open repo: %v", err)
	}
	name := plumbing.NewTagReferenceName(tag)
	ref, err := storer.Reference(name)
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: tag %s: %v", tag, err)
	}
	parent, err := repo.CommitObject(peeledCommit(repo, ref.Hash()))
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: commit of %s: %v", tag, err)
	}

	tree := treeWithMarker(t, storer, parent, fmt.Sprintf("%s moved from %s\n", tag, parent.Hash))
	sig := testAuthor()
	moved := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      "move " + tag,
		TreeHash:     tree,
		ParentHashes: []plumbing.Hash{parent.Hash},
	}
	hash := storeObject(t, storer, moved)

	if err := storer.RemoveReference(name); err != nil {
		t.Fatalf("MoveTagToNewCommit: delete tag %s: %v", tag, err)
	}
	if err := storer.SetReference(plumbing.NewHashReference(name, hash)); err != nil {
		t.Fatalf("MoveTagToNewCommit: recreate tag %s: %v", tag, err)
	}
	return hash.String()
}

// treeWithMarker writes parent's tree with moveMarkerFile set to marker.
func treeWithMarker(
	t *testing.T,
	storer *memory.Storage,
	parent *object.Commit,
	marker string,
) plumbing.Hash {
	t.Helper()

	tree, err := parent.Tree()
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: tree of %s: %v", parent.Hash, err)
	}

	blob := storer.NewEncodedObject()
	blob.SetType(plumbing.BlobObject)
	w, err := blob.Writer()
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: blob writer: %v", err)
	}
	if _, err := w.Write([]byte(marker)); err != nil {
		t.Fatalf("MoveTagToNewCommit: write blob: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("MoveTagToNewCommit: close blob: %v", err)
	}
	blobHash, err := storer.SetEncodedObject(blob)
	if err != nil {
		t.Fatalf("MoveTagToNewCommit: store blob: %v", err)
	}

	entries := make([]object.TreeEntry, 0, len(tree.Entries)+1)
	for _, e := range tree.Entries {
		if e.Name != moveMarkerFile {
			entries = append(entries, e)
		}
	}
	entries = append(entries, object.TreeEntry{Name: moveMarkerFile, Mode: filemode.Regular, Hash: blobHash})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	return storeObject(t, storer, &object.Tree{Entries: entries})
}

type encodable interface {
	Encode(plumbing.EncodedObject) error
}

func storeObject(t *testing.T, storer *memory.Storage, obj encodable) plumbing.Hash {
	t.Helper()
	encoded := storer.NewEncodedObject()
	if err := obj.Encode(encoded); err != nil {
		t.Fatalf("encode object: %v", err)
	}
	hash, err := storer.SetEncodedObject(encoded)
	if err != nil {
		t.Fatalf("store object: %v", err)
	}
	return hash
}

// testResolver implements resolver.Resolver and resolvers.ConstraintResolver
// using in-memory fixture repos. Repo I/O holds the owning set's io lock,
// the one FixtureRepos.Mutate takes, because go-git's memory.Storage is not
// safe for concurrent use.
type testResolver struct {
	repos           *FixtureRepos
	collectionRepos *FixtureRepos
	// cloneOnly refuses a manifest fetch at anything but a tag or branch
	// name, as the clone path does.
	cloneOnly bool
}

func newTestResolver(repos, collectionRepos *FixtureRepos) *testResolver {
	return &testResolver{repos: repos, collectionRepos: collectionRepos}
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
	r.repos.io.Lock()
	defer r.repos.io.Unlock()
	if err := r.admitsRef(storer, ns.Ref()); err != nil {
		return nil, "", err
	}
	if data, err := readFromRepo(storer, ns.Ref(), "ARROW.md"); err == nil {
		return data, "ARROW.md", nil
	}
	data, err := readFromRepo(storer, ns.Ref(), "arrow.yaml")
	if err != nil {
		return nil, "", err
	}
	return data, "arrow.yaml", nil
}

func (r *testResolver) ResolveArrowAt(_ context.Context, ns domain.Namespace, path string) ([]byte, string, error) {
	key := fixtureKey(ns)
	storer, ok := r.repos.Get(key)
	if !ok {
		return nil, "", fmt.Errorf("fixture repo not found: %s (key=%s)", ns, key)
	}
	r.repos.io.Lock()
	defer r.repos.io.Unlock()
	if err := r.admitsRef(storer, ns.Ref()); err != nil {
		return nil, "", err
	}
	if data, err := readFromRepo(storer, ns.Ref(), path+".md"); err == nil {
		return data, path + ".md", nil
	}
	data, err := readFromRepo(storer, ns.Ref(), path+".yaml")
	if err != nil {
		return nil, "", err
	}
	return data, path + ".yaml", nil
}

// admitsRef refuses, on a clone-only host, a ref that names no tag or
// branch: an empty ref clones the default branch.
func (r *testResolver) admitsRef(storer *memory.Storage, ref string) error {
	if !r.cloneOnly || ref == "" {
		return nil
	}
	if _, err := storer.Reference(plumbing.NewTagReferenceName(ref)); err == nil {
		return nil
	}
	if _, err := storer.Reference(plumbing.NewBranchReferenceName(ref)); err == nil {
		return nil
	}
	return fmt.Errorf("clone-only host: couldn't find remote ref %q", ref)
}

func (r *testResolver) ResolveCollection(_ context.Context, ns domain.Namespace) ([]byte, error) {
	key := strings.TrimPrefix(string(ns.BareNamespace()), "quiver.test/")
	storer, ok := r.collectionRepos.Get(key)
	if !ok {
		return nil, fmt.Errorf("collection fixture repo not found: %s (key=%s)", ns, key)
	}
	r.collectionRepos.io.Lock()
	defer r.collectionRepos.io.Unlock()
	if data, err := readFromRepo(storer, ns.Ref(), "COLLECTION.md"); err == nil {
		return data, nil
	}
	return readFromRepo(storer, ns.Ref(), "collection.yaml")
}

// Refs snapshots the fixture repo's tags (peeled to their commits), branches
// and HEAD branch, the same view the real resolver reads off a remote's ref
// advertisement.
func (r *testResolver) Refs(_ context.Context, ns domain.Namespace) (domain.RefSnapshot, error) {
	key := fixtureKey(ns)
	storer, ok := r.repos.Get(key)
	if !ok {
		return domain.RefSnapshot{}, fmt.Errorf("fixture repo not found for refs: %s", ns)
	}
	r.repos.io.Lock()
	defer r.repos.io.Unlock()
	return refSnapshotOf(storer)
}

func refSnapshotOf(storer *memory.Storage) (domain.RefSnapshot, error) {
	repo, err := gogit.Open(storer, memfs.New())
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("open repo: %w", err)
	}
	refs, err := repo.References()
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("list refs: %w", err)
	}
	defer refs.Close()

	snap := domain.RefSnapshot{Tags: map[string]string{}, Branches: map[string]string{}}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		switch {
		case ref.Name().IsBranch():
			snap.Branches[ref.Name().Short()] = ref.Hash().String()
		case ref.Name().IsTag():
			snap.Tags[ref.Name().Short()] = peeledCommit(repo, ref.Hash()).String()
		}
		return nil
	})
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("iterate refs: %w", err)
	}

	if head, err := repo.Reference(plumbing.HEAD, false); err == nil && head.Target().IsBranch() {
		if _, ok := snap.Branches[head.Target().Short()]; ok {
			snap.Head = head.Target().Short()
		}
	}
	return snap, nil
}

// peeledCommit resolves an annotated tag object to the commit it points at; a
// lightweight tag already points at the commit.
func peeledCommit(repo *gogit.Repository, hash plumbing.Hash) plumbing.Hash {
	if tag, err := repo.TagObject(hash); err == nil {
		return tag.Target
	}
	return hash
}

// testdataCollectionsDir returns the path to testdata/collections/ relative to any suite package.
func testdataCollectionsDir() string {
	return filepath.Join("..", "testdata", "collections")
}

// BuildFixtureCollectionRepos walks testdata/collections/ and builds one in-memory git repo per collection fixture.
// For each local arrow subdir found inside a collection fixture, a separate repo is registered in arrowRepos.
func BuildFixtureCollectionRepos(t *testing.T, arrowRepos *FixtureRepos) *FixtureRepos {
	t.Helper()

	root := testdataCollectionsDir()
	repos := newFixtureRepos()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("BuildFixtureCollectionRepos: readdir %s: %v", root, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		collectionDir := filepath.Join(root, name)

		storer := memory.NewStorage()
		bfs := memfs.New()
		repo, initErr := gogit.Init(storer, bfs)
		if initErr != nil {
			t.Fatalf("BuildFixtureCollectionRepos: git init for %s: %v", name, initErr)
		}

		wt, wtErr := repo.Worktree()
		if wtErr != nil {
			t.Fatalf("BuildFixtureCollectionRepos: worktree for %s: %v", name, wtErr)
		}

		_ = filepath.WalkDir(collectionDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(collectionDir, p)
			content, readErr := os.ReadFile(p) // #nosec G304 -- path is under testdata/, controlled by test fixtures only
			if readErr != nil {
				t.Fatalf("BuildFixtureCollectionRepos: read %s: %v", p, readErr)
			}
			commitFileNested(t, wt, filepath.ToSlash(relPath), content)
			return nil
		})

		hash, commitErr := wt.Commit("init", &gogit.CommitOptions{
			Author:            testAuthor(),
			AllowEmptyCommits: false,
		})
		if commitErr != nil {
			t.Fatalf("BuildFixtureCollectionRepos: commit for %s: %v", name, commitErr)
		}
		createTag(t, repo, "v1", hash)
		// Store under "quiver-test/<name>" so fixtureKey("quiver.test/quiver-test/<name>@v1") resolves correctly.
		repos.Set("quiver-test/"+name, storer)

		// Register local arrow subdirs in arrowRepos.
		_ = filepath.WalkDir(collectionDir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || !d.IsDir() || p == collectionDir {
				return nil
			}
			if !hasArrowManifestFile(p) {
				return nil
			}
			// Build key: "quiver-test/{collection-name}/{last-segment-of-subdir}"
			relDir, _ := filepath.Rel(collectionDir, p)
			segments := strings.Split(filepath.ToSlash(relDir), "/")
			last := segments[len(segments)-1]
			arrowKey := "quiver-test/" + name + "/" + last

			aStorer := memory.NewStorage()
			aFS := memfs.New()
			aRepo, aInitErr := gogit.Init(aStorer, aFS)
			if aInitErr != nil {
				t.Fatalf("BuildFixtureCollectionRepos: git init for arrow %s: %v", arrowKey, aInitErr)
			}
			aWT, aWTErr := aRepo.Worktree()
			if aWTErr != nil {
				t.Fatalf("BuildFixtureCollectionRepos: worktree for arrow %s: %v", arrowKey, aWTErr)
			}

			arrowFilename, arrowContent := readArrowManifestFile(t, p, arrowKey)
			ext := filepath.Ext(arrowFilename) // ".md" or ".yaml", whichever readArrowManifestFile found
			nestedName := filepath.ToSlash(relDir) + ext
			commitFileNested(t, aWT, nestedName, arrowContent)
			aHash, aCommitErr := aWT.Commit("init", &gogit.CommitOptions{
				Author:            testAuthor(),
				AllowEmptyCommits: false,
			})
			if aCommitErr != nil {
				t.Fatalf("BuildFixtureCollectionRepos: commit arrow %s: %v", arrowKey, aCommitErr)
			}
			createTag(t, aRepo, "v1", aHash)
			arrowRepos.Set(arrowKey, aStorer)
			return nil
		})
	}

	return repos
}

// hasArrowManifestFile returns true if dir contains ARROW.md or arrow.yaml.
func hasArrowManifestFile(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "ARROW.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "arrow.yaml")); err == nil {
		return true
	}
	return false
}

// readArrowManifestFile reads an arrow manifest from dir, preferring ARROW.md over arrow.yaml.
func readArrowManifestFile(t *testing.T, dir, key string) (string, []byte) {
	t.Helper()
	mdPath := filepath.Join(dir, "ARROW.md")
	if content, err := os.ReadFile(mdPath); err == nil { // #nosec G304 -- path is under testdata/, controlled by test fixtures only
		return "ARROW.md", content
	}
	yamlPath := filepath.Join(dir, "arrow.yaml")
	content, err := os.ReadFile(yamlPath) // #nosec G304 -- path is under testdata/, controlled by test fixtures only
	if err != nil {
		t.Fatalf("readArrowManifestFile: read %s: %v", key, err)
	}
	return "arrow.yaml", content
}

// commitFileNested stages a file at a nested path, creating parent directories in the in-memory FS.
func commitFileNested(t *testing.T, wt *gogit.Worktree, filename string, content []byte) {
	t.Helper()
	if dir := path.Dir(filename); dir != "." && dir != "" {
		if err := wt.Filesystem.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s in worktree: %v", dir, err)
		}
	}
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

// resolveFixtureRef resolves a tag, a branch, or a commit hash (full or an
// unambiguous prefix), the way a raw-file host serves a ref or a SHA.
func resolveFixtureRef(repo *gogit.Repository, ref string) (plumbing.Hash, error) {
	if tagRef, err := repo.Storer.Reference(plumbing.NewTagReferenceName(ref)); err == nil {
		return tagRef.Hash(), nil
	}
	branchRef, branchErr := repo.Storer.Reference(plumbing.NewBranchReferenceName(ref))
	if branchErr == nil {
		return branchRef.Hash(), nil
	}
	if hash, ok := commitByPrefix(repo, ref); ok {
		return hash, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("resolve ref %q: %w", ref, branchErr)
}

func commitByPrefix(repo *gogit.Repository, prefix string) (plumbing.Hash, bool) {
	if len(prefix) < 7 {
		return plumbing.ZeroHash, false
	}
	iter, err := repo.CommitObjects()
	if err != nil {
		return plumbing.ZeroHash, false
	}
	defer iter.Close()

	var found plumbing.Hash
	matches := 0
	_ = iter.ForEach(func(c *object.Commit) error {
		if strings.HasPrefix(c.Hash.String(), strings.ToLower(prefix)) {
			found = c.Hash
			matches++
		}
		return nil
	})
	return found, matches == 1
}

func readFromRepo(storer *memory.Storage, ref, filename string) ([]byte, error) {
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
		commitHash, err = resolveFixtureRef(repo, ref)
		if err != nil {
			return nil, err
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
