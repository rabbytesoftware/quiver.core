package reader

import (
	"github.com/rabbytesoftware/quiver/internal/infrastructure/translator/parser"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/quiver"
)

type Reader interface {
	ReadArrow(manifestPath string) (*arrow.Arrow, error)
	ReadQuiver(manifestPath string) (*quiver.Quiver, error)
	ReadManifestInfo(manifestPath string) (*parser.ManifestInfo, error)
}
