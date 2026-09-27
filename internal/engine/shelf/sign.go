package shelf

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func (s *shelf) sign(
	ctx context.Context,
	workdir string,
	target string,
) {
	if s.goos != goosDarwin || s.goarch != goarchARM64 || insideBundle(target) {
		return
	}
	if !resolvesInside(workdir, target) || !isMachO(target) {
		return
	}
	if _, err := s.commander.Run(ctx, "codesign", "-v", target); err == nil {
		return
	}

	out, err := s.commander.Run(ctx, "codesign", "-s", "-", "-f", target)
	if err != nil {
		slog.WarnContext(ctx, "shelf: ad-hoc signing failed", "target", target, "err", err, "output", string(out))
	}
}

func insideBundle(
	target string,
) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(target)), "/") {
		if strings.HasSuffix(part, bundleExtension) {
			return true
		}
	}
	return false
}

func isMachO(
	path string,
) bool {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		return false
	}

	for _, known := range [][]byte{
		{0xfe, 0xed, 0xfa, 0xce},
		{0xfe, 0xed, 0xfa, 0xcf},
		{0xce, 0xfa, 0xed, 0xfe},
		{0xcf, 0xfa, 0xed, 0xfe},
		{0xca, 0xfe, 0xba, 0xbe},
	} {
		if bytes.Equal(magic, known) {
			return true
		}
	}
	return false
}
