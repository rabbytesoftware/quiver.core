package build

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

// Digest returns the SHA-256 digest of the file at path.
func Digest(
	ctx context.Context,
	path string,
) (string, error) {
	reader, err := fns.ReadStream(ctx, path)
	if err != nil {
		return "", fmt.Errorf("build digest: open executable: %w", err)
	}
	digest, err := digestReader(ctx, reader)
	if err != nil {
		return "", fmt.Errorf("build digest: %w", err)
	}
	return digest, nil
}

func digestReader(ctx context.Context, reader io.ReadCloser) (string, error) {
	hash := sha256.New()
	_, copyErr := io.Copy(hash, &contextReader{ctx: ctx, reader: reader})
	closeErr := reader.Close()
	if copyErr != nil {
		return "", fmt.Errorf("read executable: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close executable: %w", closeErr)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}
