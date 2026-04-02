package vault

import "github.com/rabbytesoftware/quiver/internal/domain"

const (
	arrowFilename  = "arrow.json"
	quiverFilename = "quiver.json"
)

type Vault interface {
	GetArrow(
		namespace domain.Namespace,
	) (*domain.ArrowManifest, string, error)

	GetQuiver(
		namespace domain.Namespace,
	) (*domain.QuiverManifest, string, error)

	PutArrow(
		namespace domain.Namespace,
		manifest *domain.ArrowManifest,
	) (string, error)

	PutQuiver(
		namespace domain.Namespace,
		manifest *domain.QuiverManifest,
	) (string, error)

	DeleteArrow(
		namespace domain.Namespace,
	) error

	DeleteQuiver(
		namespace domain.Namespace,
	) error
}
