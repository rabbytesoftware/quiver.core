package desktop

import (
	"context"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

type Placer interface {
	Place(
		ctx context.Context,
		req models.ApplyRequest,
		entry domain.ExposeEntry,
		c models.Candidate,
	) (models.Placement, error)
	Remove(
		ctx context.Context,
		l platform.Layout,
		claim ownership.Claim,
		keep map[string]bool,
	) error
}

type placer struct {
	host    platform.Host
	bundles ownership.Bundles
}

func New(
	host platform.Host,
	bundles ownership.Bundles,
) Placer {
	return &placer{host: host, bundles: bundles}
}

func (p *placer) Place(
	ctx context.Context,
	req models.ApplyRequest,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	switch p.host.GOOS {
	case platform.GOOSDarwin:
		return p.placeApp(ctx, req, c)
	case platform.GOOSWindows:
		return p.placeLnk(ctx, req, entry, c)
	}
	return placeXDG(req, entry, c)
}

func (p *placer) Remove(
	ctx context.Context,
	l platform.Layout,
	claim ownership.Claim,
	keep map[string]bool,
) error {
	switch p.host.GOOS {
	case platform.GOOSDarwin:
		return p.removeApps(l, claim, keep)
	case platform.GOOSWindows:
		return p.removeLnks(ctx, l, claim, keep)
	}
	return removeXDG(l, claim, keep)
}

func desktopIcon(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
	c models.Candidate,
) string {
	icon := req.Media.Icon
	if c.Icon != "" {
		icon = c.Icon
	}
	if entry.Icon != "" {
		icon = fsguard.ExpandPath(entry.Icon, req.Workdir)
	}
	if strings.Contains(icon, "://") || fsguard.UnsafePath(icon, "") {
		return ""
	}
	return icon
}
