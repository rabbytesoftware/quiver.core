package collection

import "github.com/rabbytesoftware/quiver.core/internal/domain"

type Module interface {
	Version() string
	Map(data []byte) (*domain.Collection, []domain.CollectionArrowEntry, error)
	GetSchema() ([]byte, error)
}
