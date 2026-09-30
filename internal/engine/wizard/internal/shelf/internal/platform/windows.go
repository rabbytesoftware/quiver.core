package platform

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/cli"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/desktop/lnk"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/pathenv"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
)

func windows(
	h host.Host,
	seams Seams,
) Platform {
	scan := discover.Scan{Rule: discover.ExeSuffix()}
	editor := userpath.NewEditor(seams.UserPath)
	return Platform{
		CLI:      cli.NewPathDir(scan, editor),
		Desktop:  lnk.New(scan),
		Path:     pathenv.NewWindows(h, editor),
		SafeName: windowsSafeName,
	}
}

func windowsSafeName(
	name string,
) bool {
	if !fsguard.SafeName(name) || domain.IsWindowsReservedName(name) {
		return false
	}
	return !strings.HasSuffix(name, ".") && !strings.HasSuffix(name, " ")
}
