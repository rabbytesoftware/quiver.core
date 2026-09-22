package resolvers

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Fetcher interface {
	CanResolve(
		namespace domain.Namespace,
	) bool

	// Fetch tries every candidate in filePaths against namespace and
	// reports which one matched. Trying every candidate is the fetcher's
	// own job, not the caller's: an implementation whose fetch step is
	// expensive (a git clone) pays for it once per call, not once per
	// candidate filename.
	Fetch(
		ctx context.Context,
		namespace domain.Namespace,
		filePaths []string,
		timeout time.Duration,
	) (data []byte, matchedPath string, err error)
}
