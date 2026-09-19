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
// its declared checksum. The downloaded file is removed before this is
// returned — a corrupted or tampered download must never be left on disk
// where a later step could act on it.
var ErrChecksumMismatch = errors.New("download: checksum mismatch")

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

	if err := fns.Download(stepCtx, url, dst, nil, downloadOpts...); err != nil {
		return err
	}

	checksum := s.Checksum.Resolve(req.OSArch.String())
	if checksum == "" {
		return nil
	}

	if err := verifyChecksum(dst, checksum); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}

func verifyChecksum(
	path string,
	want string,
) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("download: checksum: open %s: %w", path, err)
	}
	defer f.Close()

	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return fmt.Errorf("download: checksum: read %s: %w", path, err)
	}

	got := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("download: checksum: %s: expected %s, got %s: %w", path, want, got, ErrChecksumMismatch)
	}
	return nil
}
