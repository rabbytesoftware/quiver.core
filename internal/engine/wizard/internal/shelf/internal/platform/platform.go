package platform

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
)

type Platform struct {
	CLI      models.Exposer
	Desktop  models.Exposer
	Path     models.PathManager
	SafeName func(name string) bool
}

type Seams struct {
	Tagger   ownership.Tagger
	UserPath userpath.UserPath
}

func ForOS(
	goos string,
	h host.Host,
	seams Seams,
) Platform {
	switch goos {
	case "darwin":
		return darwin(h, seams)
	case "windows":
		return windows(h, seams)
	}
	return linux(h)
}
