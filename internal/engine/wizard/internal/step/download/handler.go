package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

// ErrChecksumMismatch means a fetch step's downloaded content did not match
// its declared checksum. The download is staged beside its destination and
// removed before this is returned — a corrupted or tampered download never
// reaches the destination, where a later step could act on it.
var ErrChecksumMismatch = errors.New("download: checksum mismatch")

// ErrChecksumUnresolved means a fetch step declared its checksum as a
// variable reference (contains "${") but that reference resolved to an
// empty value — distinct from a manifest that declares no checksum at all.
// Treating an unresolved reference the same as "no checksum declared" would
// silently disable verification for exactly the case a template checksum
// exists to guard: the caller failed to supply the value, not chose to skip
// checking. The staged download is removed before this is returned, same as
// ErrChecksumMismatch.
var ErrChecksumUnresolved = errors.New("download: checksum: variable reference resolved to an empty value")

var ErrUnsupportedChecksumAlgorithm = errors.New("download: unsupported checksum algorithm")

const checksumAlgorithmSHA256 = "sha256"

type handler struct{}

func NewHandler() wizstep.Handler[domainstep.FetchStep] {
	return &handler{}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.FetchStep,
) error {
	stepCtx := ctx
	var cancel context.CancelFunc

	var downloadOpts []config.Option

	if ts := s.Timeout.Resolve(req.OSArch.String()); ts != "" {
		d, err := time.ParseDuration(ts)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", ts, err)
		}

		stepCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()

		// Disable the HTTP client's own Timeout so the context deadline is the
		// sole authority. Without this, the default 30s client timeout fires
		// before any step timeout longer than 30s can take effect.
		downloadOpts = append(downloadOpts, config.WithTimeout(0))
	}

	dst := req.Expand(s.To.Resolve(req.OSArch.String()))
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(req.WorkDir, dst)
	}

	url := req.Expand(s.URL.Resolve(req.OSArch.String()))

	staged, err := stagingPath(dst)
	if err != nil {
		return err
	}
	defer os.Remove(staged) //nolint:errcheck // gone already once it replaced dst

	if err := fns.Download(stepCtx, url, staged, nil, downloadOpts...); err != nil {
		return err
	}
	if err := h.verify(stepCtx, req, s, staged); err != nil {
		return err
	}
	keepMode(staged, dst)
	if err := replaceFile(staged, dst, os.Rename); err != nil {
		return err
	}
	sweepStale(dst)
	return nil
}

// verify checks the staged download against the step's checksum, if it
// declares one. A download that fails it never reaches the destination, so
// whatever the destination held before stays.
func (h *handler) verify(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.FetchStep,
	staged string,
) error {
	rawChecksum := s.Checksum.Resolve(req.OSArch.String())
	checksum := req.Expand(rawChecksum)
	if checksum != "" {
		return verifyChecksum(ctx, staged, checksum)
	}
	if strings.Contains(rawChecksum, "${") {
		return fmt.Errorf("download: checksum: %q: %w", rawChecksum, ErrChecksumUnresolved)
	}
	return nil
}

func verifyChecksum(
	ctx context.Context,
	path string,
	want string,
) error {
	expected, err := expectedDigest(want)
	if err != nil {
		return err
	}

	rc, err := fns.ReadStream(ctx, path)
	if err != nil {
		return fmt.Errorf("download: checksum: open %s: %w", path, err)
	}
	defer rc.Close() //nolint:errcheck

	digest := sha256.New()
	if _, err := io.Copy(digest, rc); err != nil {
		return fmt.Errorf("download: checksum: read %s: %w", path, err)
	}

	got := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("download: checksum: %s: expected %s, got %s: %w", path, expected, got, ErrChecksumMismatch)
	}
	return nil
}

func expectedDigest(
	checksum string,
) (string, error) {
	checksum = strings.TrimSpace(checksum)
	algorithm, digest, tagged := strings.Cut(checksum, ":")
	if !tagged {
		return checksum, nil
	}
	if !strings.EqualFold(algorithm, checksumAlgorithmSHA256) {
		return "", fmt.Errorf("download: checksum: %q: %w", algorithm, ErrUnsupportedChecksumAlgorithm)
	}
	return digest, nil
}
