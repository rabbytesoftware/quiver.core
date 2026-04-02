package resolvers

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Fetcher interface {
	CanResolve(namespace domain.Namespace) bool
	Fetch(
		ctx       context.Context,
		namespace domain.Namespace,
		filePath  string,
		timeout   time.Duration,
	) ([]byte, error)
}
