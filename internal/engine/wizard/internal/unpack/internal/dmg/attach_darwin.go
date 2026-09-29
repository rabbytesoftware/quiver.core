package dmg

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const detachTimeout = 30 * time.Second

func hdiutil() mounter {
	return mounter{attach: attach, detach: detach}
}

func attach(
	ctx context.Context,
	image string,
	mount string,
) error {
	cmd := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-noautoopen", "-readonly", "-mountpoint", mount, image) // #nosec G204 -- fixed hdiutil binary; image is a workdir-resolved path and mount is a Quiver temp dir
	cmd.Stdin = strings.NewReader("Y\n")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unpack: dmg: attach %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func detach(
	ctx context.Context,
	mount string,
) {
	detachCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), detachTimeout)
	defer cancel()

	out, err := exec.CommandContext(detachCtx, "hdiutil", "detach", mount, "-force").CombinedOutput() // #nosec G204 -- fixed hdiutil binary; mount is a Quiver temp dir
	if err != nil {
		slog.WarnContext(ctx, "unpack: dmg: detach failed", "mount", mount, "err", err, "output", string(out))
	}
}
