package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

type Input struct {
	Repo        string
	Name        string
	Description string
	URL         string
	Media       domain.ArrowMedia
	Readme      string
	Generator   domain.ArrowGenerator
	Picks       map[domain.OS]picker.Pick
}
