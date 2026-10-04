package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
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

// ErrChecksumEntryMissing means a sha256sums checksum named a file its
// published list does not contain. It fails closed: a list that does not vouch
// for the download is not a reason to accept it.
var ErrChecksumEntryMissing = errors.New("download: checksum: entry not in the published sums")

// ErrUnsupportedURL means a fetch step's URL is not http or https. Other
// schemes would reach the local file strategy and copy from the machine
// instead of downloading.
var ErrUnsupportedURL = errors.New("download: only http and https urls can be fetched")

const (
	checksumAlgorithmSHA256 = "sha256"
	sha256SumsPrefix        = "sha256sums:"
)

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

	src := req.Expand(s.URL.Resolve(req.OSArch.String()))
	if err := requireHTTP(src); err != nil {
		return err
	}

	staged, err := stagingPath(dst)
	if err != nil {
		return err
	}
	defer os.Remove(staged) //nolint:errcheck // gone already once it replaced dst

	if err := fns.Download(stepCtx, src, staged, nil, downloadOpts...); err != nil {
		return err
	}
	if err := h.verify(stepCtx, req, s, src, staged); err != nil {
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
	src string,
	staged string,
) error {
	rawChecksum := s.Checksum.Resolve(req.OSArch.String())
	checksum := req.Expand(rawChecksum)
	if checksum != "" {
		return verifyChecksum(ctx, staged, checksum, src)
	}
	if strings.Contains(rawChecksum, "${") {
		return fmt.Errorf("download: checksum: %q: %w", rawChecksum, ErrChecksumUnresolved)
	}
	return nil
}

func verifyChecksum(
	ctx context.Context,
	file string,
	want string,
	src string,
) error {
	expected, err := expectedDigest(ctx, want, src)
	if err != nil {
		return err
	}

	rc, err := fns.ReadStream(ctx, file)
	if err != nil {
		return fmt.Errorf("download: checksum: open %s: %w", file, err)
	}
	defer rc.Close() //nolint:errcheck

	digest := sha256.New()
	if _, err := io.Copy(digest, rc); err != nil {
		return fmt.Errorf("download: checksum: read %s: %w", file, err)
	}

	got := hex.EncodeToString(digest.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("download: checksum: %s: expected %s, got %s: %w", file, expected, got, ErrChecksumMismatch)
	}
	return nil
}

// expectedDigest turns a declared checksum into the hex digest to compare: a
// bare hex digest, sha256:<hex>, or sha256sums:<url>[#<entry>] naming a
// published sums list, where the entry defaults to the file name of the
// fetched URL.
func expectedDigest(
	ctx context.Context,
	checksum string,
	src string,
) (string, error) {
	checksum = strings.TrimSpace(checksum)
	if list, ok := strings.CutPrefix(checksum, sha256SumsPrefix); ok {
		return publishedDigest(ctx, list, src)
	}
	algorithm, digest, tagged := strings.Cut(checksum, ":")
	if !tagged {
		return checksum, nil
	}
	if !strings.EqualFold(algorithm, checksumAlgorithmSHA256) {
		return "", fmt.Errorf("download: checksum: %q: %w", algorithm, ErrUnsupportedChecksumAlgorithm)
	}
	return digest, nil
}

func publishedDigest(
	ctx context.Context,
	list string,
	src string,
) (string, error) {
	sumsURL, entry, _ := strings.Cut(list, "#")
	if err := requireHTTP(sumsURL); err != nil {
		return "", err
	}
	if entry == "" {
		parsed, err := url.Parse(src)
		if err != nil {
			return "", fmt.Errorf("download: checksum: parse %q: %w", src, err)
		}
		entry = path.Base(parsed.Path)
	}

	body, err := fns.Fetch(ctx, sumsURL)
	if err != nil {
		return "", fmt.Errorf("download: checksum: fetch %s: %w", sumsURL, err)
	}

	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(fields[1], "*"), "./")
		if name == strings.TrimPrefix(entry, "./") {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("download: checksum: %s: %q: %w", sumsURL, entry, ErrChecksumEntryMissing)
}

func requireHTTP(
	raw string,
) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("download: %q: %w", raw, ErrUnsupportedURL)
	}
	return nil
}
