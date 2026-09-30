package recommendationinternal

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
)

// Browser is the part of discovery a refresh needs.
type Browser interface {
	Browse(
		ctx context.Context,
		req discovery.BrowseRequest,
		emit func(discovery.Result),
	) (discovery.Outcome, error)
}
