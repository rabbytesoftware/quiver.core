package recommendation

import (
	"context"
	"time"
)

func Due(
	r Recommendation,
	ctx context.Context,
) bool {
	return r.(*recommendation).due(ctx)
}

func ParseWindow(
	raw string,
) (time.Duration, error) {
	return parseWindow(raw)
}
