package resolvers

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver/internal/domain"
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
	filePath string,
	timeout time.Duration,
) ([]byte, error) {
	return fetchFile(
		ctx,
		namespace.BareNamespace().CloneURL(),
		filePath,
		timeout,
		namespace.Ref(),
	)
}

func fetchFile(
	ctx context.Context,
	cloneURL string,
	filePath string,
	timeout time.Duration,
	ref string,
) ([]byte, error) {
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

	repo, err := gogit.CloneContext(ctx, storer, fs, opts)
	if err != nil && ref != "" {
		// retry as branch ref
		fs = memfs.New()
		storer = memory.NewStorage()
		opts.ReferenceName = plumbing.NewBranchReferenceName(ref)
		repo, err = gogit.CloneContext(ctx, storer, fs, opts)
	}
	if err != nil {
		return nil, wrapFetchErr(err, cloneURL)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("%w: worktree: %v", ErrFetchFailed, err)
	}

	f, err := wt.Filesystem.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %v", ErrNotFound, filePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrFetchFailed, filePath, err)
	}

	return data, nil
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
