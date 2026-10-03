package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

// ErrStagedUnusable means a staged binary is not the file that was verified
// when it was staged: missing, empty, truncated or replaced.
var ErrStagedUnusable = errors.New("selfupdate: staged binary is unusable")

// Fingerprint measures the file at path: its size and SHA-256.
func Fingerprint(
	ctx context.Context,
	path string,
) (int64, string, error) {
	rc, err := fns.ReadStream(ctx, path)
	if err != nil {
		return 0, "", fmt.Errorf("selfupdate: fingerprint %s: %w", path, err)
	}
	defer rc.Close() //nolint:errcheck

	digest := sha256.New()
	size, err := io.Copy(digest, rc)
	if err != nil {
		return 0, "", fmt.Errorf("selfupdate: fingerprint %s: read: %w", path, err)
	}
	return size, hex.EncodeToString(digest.Sum(nil)), nil
}

// Verify reports whether the file at path still has the size and SHA-256
// recorded for it when it was staged. A binary that cannot be told apart from
// a different one, or that is empty, is never usable.
func Verify(
	ctx context.Context,
	path string,
	size int64,
	digest string,
) error {
	if size <= 0 || strings.TrimSpace(digest) == "" {
		return fmt.Errorf("%w: %s: nothing recorded to compare it with", ErrStagedUnusable, path)
	}
	gotSize, gotDigest, err := Fingerprint(ctx, path)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStagedUnusable, err)
	}
	if gotSize != size || !strings.EqualFold(gotDigest, strings.TrimSpace(digest)) {
		return fmt.Errorf("%w: %s: has %d bytes and digest %s, staged with %d bytes and digest %s", ErrStagedUnusable, path, gotSize, gotDigest, size, digest)
	}
	return nil
}
