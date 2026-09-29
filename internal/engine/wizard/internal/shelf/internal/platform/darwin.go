package platform

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/cli"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/desktop/bundle"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/pathenv"
)

func darwin(
	h host.Host,
	seams Seams,
) Platform {
	bundles := ownership.NewBundles(seams.Tagger)
	scan := discover.Scan{Rule: discover.ExecBits(), Opaque: discover.IsBundle}
	return Platform{
		CLI:      cli.NewSymlink(h, scan, darwinSigner(h), bundles.Enclosing),
		Desktop:  bundle.New(h, bundles),
		Path:     pathenv.NewRC(h, "zsh", []string{".bashrc", ".bash_profile"}),
		SafeName: fsguard.SafeName,
	}
}

// darwinSigner ad-hoc signs on Apple silicon only, the one Mac that refuses to
// launch unsigned code.
func darwinSigner(
	h host.Host,
) cli.Signer {
	if h.GOARCH == "arm64" {
		return cli.Codesign(h.Commander)
	}
	return cli.Unsigned()
}
