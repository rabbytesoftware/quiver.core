package step

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

func (r Request) ResolvePath(
	raw string,
) string {
	path := r.Expand(raw)
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(r.WorkDir, path)
}

func WithTimeout(
	ctx context.Context,
	raw string,
) (context.Context, context.CancelFunc, error) {
	if raw == "" {
		stepCtx, cancel := context.WithCancel(ctx)
		return stepCtx, cancel, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid timeout %q: %w", raw, err)
	}

	stepCtx, cancel := context.WithTimeout(ctx, d)

	return stepCtx, cancel, nil
}
