package platform

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/cli"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/desktop/xdg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/pathenv"
)

func linux(
	h host.Host,
) Platform {
	scan := discover.Scan{Rule: discover.ExecBits()}
	return Platform{
		CLI:      cli.NewSymlink(h, scan, cli.Unsigned(), ownership.Unowned),
		Desktop:  xdg.New(h, scan),
		Path:     pathenv.NewRC(h, "", []string{".bashrc"}),
		SafeName: fsguard.SafeName,
	}
}
