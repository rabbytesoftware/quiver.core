package quiver

import "github.com/rabbytesoftware/quiver/internal/domain"

type Module interface {
	Version() string
	Map(data []byte) (*domain.QuiverManifest, error)
	GetSchema() ([]byte, error)
}
