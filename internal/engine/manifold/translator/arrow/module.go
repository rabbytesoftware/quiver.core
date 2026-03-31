package arrow

import "github.com/rabbytesoftware/quiver/internal/domain"

type Module interface {
	Version() string
	Map(data []byte) (*domain.ArrowManifest, error)
	GetSchema() ([]byte, error)
}
