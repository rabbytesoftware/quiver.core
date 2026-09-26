package resolvers

import (
	"context"
	"fmt"
	"io"
	"time"

	billy "github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type gitFetcher struct{}

func NewGit() Fetcher {
	return &gitFetcher{}
}

func (g *gitFetcher) CanResolve(
	_ domain.Namespace,
) bool {
	return true
}

func (g *gitFetcher) Fetch(
	ctx context.Context,
	namespace domain.Namespace,
	filePaths []string,
	timeout time.Duration,
) ([]byte, string, error) {
	return fetchFile(
		ctx,
		namespace.BareNamespace().CloneURL(),
		filePaths,
		timeout,
		namespace.Ref(),
		cloneRepo,
	)
}

// cloneFn matches gogit.CloneContext's signature. fetchFile takes it as a
// parameter, rather than calling gogit.CloneContext directly, purely so a
// test can inject a counting or fake clone without a real network clone —
// production always passes cloneRepo.
type cloneFn func(
	ctx context.Context,
	storer storage.Storer,
	worktree billy.Filesystem,
	o *gogit.CloneOptions,
) (*gogit.Repository, error)

// cloneRepo is cloneFn's real implementation.
func cloneRepo(
	ctx context.Context,
	storer storage.Storer,
	worktree billy.Filesystem,
	o *gogit.CloneOptions,
) (*gogit.Repository, error) {
	return gogit.CloneContext(ctx, storer, worktree, o)
}

// fetchFile clones cloneURL exactly once, then checks every candidate in
// filePaths against that same clone, in order, returning the first one that
// opens. Trying every candidate against one already-cloned worktree, rather
// than cloning once per candidate, is the whole point: a git clone is the
// expensive part of manifest resolution, and a ref carrying none of the
// candidates (an old tag cut before the manifest convention existed, say)
// must not pay for a clone once per candidate name just to find that out.
func fetchFile(
	ctx context.Context,
	cloneURL string,
	filePaths []string,
	timeout time.Duration,
	ref string,
	clone cloneFn,
) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fs := memfs.New()
	storer := memory.NewStorage()

	opts := &gogit.CloneOptions{
		URL:   cloneURL,
		Depth: 1,
	}

	if ref != "" {
		opts.ReferenceName = plumbing.NewTagReferenceName(ref)
		opts.SingleBranch = true
	}

	repo, err := clone(ctx, storer, fs, opts)
	if err != nil && ref != "" {
		// retry as branch ref
		fs = memfs.New()
		storer = memory.NewStorage()
		opts.ReferenceName = plumbing.NewBranchReferenceName(ref)
		repo, err = clone(ctx, storer, fs, opts)
	}
	if err != nil {
		return nil, "", wrapFetchErr(err, cloneURL)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, "", fmt.Errorf("%w: worktree: %v", ErrFetchFailed, err)
	}

	return openFirstMatch(wt.Filesystem, filePaths)
}

// openFirstMatch tries each candidate in filePaths, in order, against fs,
// returning the first one that opens. The not-found error names every
// candidate tried, since no single one of them "not found" tells the whole
// story of what was attempted.
func openFirstMatch(
	fs billy.Filesystem,
	filePaths []string,
) ([]byte, string, error) {
	for _, filePath := range filePaths {
		f, err := fs.Open(filePath)
		if err != nil {
			continue
		}

		data, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil {
			return nil, "", fmt.Errorf("%w: read %s: %v", ErrFetchFailed, filePath, readErr)
		}
		if closeErr != nil {
			return nil, "", fmt.Errorf("%w: close %s: %v", ErrFetchFailed, filePath, closeErr)
		}
		return data, filePath, nil
	}

	return nil, "", fmt.Errorf("%w: none of %v found", ErrNotFound, filePaths)
}

func wrapFetchErr(
	err error,
	cloneURL string,
) error {
	if err == transport.ErrRepositoryNotFound {
		return fmt.Errorf("%w: repository not found: %s", ErrNotFound, cloneURL)
	}

	return fmt.Errorf("%w: clone %s: %v", ErrFetchFailed, cloneURL, err)
}
