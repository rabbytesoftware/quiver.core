package cli

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

func (p *placer) sign(
	ctx context.Context,
	workdir string,
	target string,
) {
	if p.host.GOOS != platform.GOOSDarwin || p.host.GOARCH != platform.GOARCHARM64 || insideBundle(target) {
		return
	}
	if _, ok := fsguard.ResolveInside(workdir, target); !ok || !isMachO(target) {
		return
	}
	if _, err := p.host.Commander.Run(ctx, nil, "codesign", "-v", target); err == nil {
		return
	}

	out, err := p.host.Commander.Run(ctx, nil, "codesign", "-s", "-", "-f", target)
	if err != nil {
		slog.WarnContext(ctx, "shelf: ad-hoc signing failed", "target", target, "err", err, "output", string(out))
	}
}

func insideBundle(
	target string,
) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Dir(target)), "/") {
		if strings.HasSuffix(part, platform.BundleExt) {
			return true
		}
	}
	return false
}

func isMachO(
	path string,
) bool {
	f, err := os.Open(path) // #nosec G304 -- path is a codesign target already confined to the workdir by fsguard.ResolveInside
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
