package media

import (
	"context"
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const (
	minRasterIconSide = 128
	iconProbeWorkers  = 24
	squareTolerance   = 0.02
	noHit             = math.MaxInt64
)

// ProbeIcon looks for the repository's own icon among well-known raw-file
// paths, pinned to ref first and to the host's default branches after it. The
// candidates are fetched concurrently but ranked ref-major, path-minor: the
// answer is the first candidate in that order that qualifies, and a hit stops
// every lower-ranked candidate from being fetched. It returns "" when none
// qualifies.
func ProbeIcon(
	ctx context.Context,
	fetch Fetch,
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) string {
	urls := candidateURLs(host, ns, ref)
	best := probeRanked(ctx, fetch, urls)
	if best == noHit {
		return ""
	}
	return urls[best]
}

func candidateURLs(
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) []string {
	refs := slices.Compact(append([]string{ref}, host.DefaultBranches()...))
	paths := iconProbePaths()
	urls := make([]string, 0, len(refs)*len(paths))
	for _, candidate := range refs {
		urls = append(urls, pinnedURLs(fileURLsOf(host, ns, candidate), paths)...)
	}
	return urls
}

func pinnedURLs(
	files fileURLs,
	paths []string,
) []string {
	urls := make([]string, 0, len(paths))
	for _, path := range paths {
		url, ok := files.pinned(path)
		if !ok {
			return nil
		}
		urls = append(urls, url)
	}
	return urls
}

func probeRanked(
	ctx context.Context,
	fetch Fetch,
	urls []string,
) int64 {
	var best atomic.Int64
	best.Store(noHit)

	jobs := make(chan int64)
	var wg sync.WaitGroup
	for range iconProbeWorkers {
		wg.Add(1)
		go probeWorker(ctx, fetch, urls, jobs, &best, &wg)
	}
	for i := range urls {
		if int64(i) > best.Load() {
			break
		}
		jobs <- int64(i)
	}
	close(jobs)
	wg.Wait()
	return best.Load()
}

func probeWorker(
	ctx context.Context,
	fetch Fetch,
	urls []string,
	jobs <-chan int64,
	best *atomic.Int64,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	for rank := range jobs {
		probeRankedOne(ctx, fetch, urls[rank], rank, best)
	}
}

func probeRankedOne(
	ctx context.Context,
	fetch Fetch,
	url string,
	rank int64,
	best *atomic.Int64,
) {
	if rank > best.Load() {
		return
	}
	data, ok := fetchProbe(ctx, fetch, url)
	if !ok || !isIconShaped(data) {
		return
	}
	for {
		current := best.Load()
		if rank >= current || best.CompareAndSwap(current, rank) {
			return
		}
	}
}

func isIconShaped(
	data []byte,
) bool {
	dim, ok := sniff(data)
	if !ok || !isSquareDim(dim) {
		return false
	}
	return dim.Vector || dim.Width >= minRasterIconSide
}

func isSquareDim(
	dim dimensions,
) bool {
	larger := max(dim.Width, dim.Height)
	smaller := min(dim.Width, dim.Height)
	return larger-smaller <= larger*squareTolerance
}
