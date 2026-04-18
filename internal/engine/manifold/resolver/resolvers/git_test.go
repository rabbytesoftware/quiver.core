package resolvers

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func makeTaggedRepo(
	t *testing.T,
	filename string,
	content []byte,
	tagName string,
) string {
	t.Helper()

	dir := makeLocalRepo(t, filename, content)

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	if _, err := repo.CreateTag(tagName, head.Hash(), nil); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	return dir
}

// makeLocalRepo initialises a temporary git repo, writes filename with content,
// commits it, and returns the repo directory path.
func makeLocalRepo(
	t *testing.T,
	filename string,
	content []byte,
) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	p := dir + "/" + filename
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("Add: %v", err)
	}

	_, err = wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return dir
}

// ─── GitFetcher ────────────────────────────────────────────────────────────────

func TestGitFetcher_CanResolve_AlwaysTrue(t *testing.T) {
	fetcher := NewGit()

	ok := fetcher.CanResolve(domain.Namespace("github.com/user/repo"))
	if !ok {
		t.Error("CanResolve() = false, want true")
	}

	ok = fetcher.CanResolve(domain.Namespace("unknown.example.com/user/repo"))
	if !ok {
		t.Error("CanResolve() = false, want true")
	}
}

// TestGitFetcher_Fetch_ReturnsErrorForUnreachableRemote exercises the public
// Fetch method. CloneURL() always constructs an https:// URL so we use a
// namespace whose domain is unreachable; the method must return a non-nil error.
func TestGitFetcher_Fetch_ReturnsErrorForUnreachableRemote(t *testing.T) {
	fetcher := NewGit()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := fetcher.Fetch(
		ctx,
		domain.Namespace("localhost/user/nonexistent"),
		"arrow.yaml",
		2*time.Second,
	)
	if err == nil {
		t.Fatal("Fetch() expected error for unreachable remote, got nil")
	}
}

// ─── fetchFile ────────────────────────────────────────────────────────────────

func TestFetchFile_HappyPath(t *testing.T) {
	content := []byte("schema: arrow@v0\n")
	dir := makeLocalRepo(t, "arrow.yaml", content)

	data, err := fetchFile(
		context.Background(),
		dir,
		"arrow.yaml",
		5*time.Second,
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("data = %q, want %q", data, content)
	}
}

func TestFetchFile_FileNotFound(t *testing.T) {
	dir := makeLocalRepo(t, "arrow.yaml", []byte("content"))

	_, err := fetchFile(
		context.Background(),
		dir,
		"nonexistent.yaml",
		5*time.Second,
		"",
	)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFetchFile_CloneError(t *testing.T) {
	_, err := fetchFile(
		context.Background(),
		"/nonexistent/path/repo",
		"arrow.yaml",
		5*time.Second,
		"",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent clone path")
	}
	if !errors.Is(err, ErrFetchFailed) && !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrFetchFailed or ErrNotFound, got %v", err)
	}
}

func TestFetchFile_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, err := fetchFile(
		ctx,
		"https://github.com/rabbytesoftware/nonexistent-repo",
		"arrow.yaml",
		30*time.Second,
		"",
	)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchFile_WithTagRef_Success(t *testing.T) {
	content := []byte("schema: arrow@v0\n")
	dir := makeTaggedRepo(t, "arrow.yaml", content, "v1.0.0")

	data, err := fetchFile(
		context.Background(),
		dir,
		"arrow.yaml",
		5*time.Second,
		"v1.0.0",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("data = %q, want %q", data, content)
	}
}

// TestFetchFile_BranchRetry verifies that when a ref exists as a branch (not a
// tag), fetchFile retries after the tag clone fails and succeeds via the branch
// reference. go-git PlainInit creates the commit on "master" by default.
func TestFetchFile_BranchRetry(t *testing.T) {
	content := []byte("schema: arrow@v0\n")
	dir := makeLocalRepo(t, "arrow.yaml", content)

	// "master" is a branch, not a tag — tag clone fails, branch retry succeeds.
	data, err := fetchFile(
		context.Background(),
		dir,
		"arrow.yaml",
		5*time.Second,
		"master",
	)
	if err != nil {
		t.Fatalf("branch retry: unexpected error: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("data = %q, want %q", data, content)
	}
}

// ─── wrapFetchErr ─────────────────────────────────────────────────────────────

func TestWrapFetchErr_GenericError(t *testing.T) {
	err := wrapFetchErr(
		errors.New("connection refused"),
		"https://github.com/user/repo",
	)
	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("expected ErrFetchFailed, got %v", err)
	}
}

func TestWrapFetchErr_RepositoryNotFound(t *testing.T) {
	err := wrapFetchErr(
		transport.ErrRepositoryNotFound,
		"https://github.com/user/repo",
	)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
