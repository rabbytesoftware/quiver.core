package quiver

import "github.com/rabbytesoftware/quiver/internal/domain"

type Module interface {
	Version() string
	Map(data []byte) (*domain.Quiver, []domain.QuiverArrowEntry, error)
	GetSchema() ([]byte, error)
}
