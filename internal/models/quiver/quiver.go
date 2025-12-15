package quiver

import (
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

type Quiver struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	Banner          shared.URL         `json:"banner"`
	URL             shared.URL         `json:"url"`
	Security        shared.Security    `json:"security"`
	Maintainers     []string           `json:"maintainers"`
	Version         string             `json:"version"`
	InstalledArrows []arrow.Arrow      `json:"installed_arrows"`
	ListedArrows    []shared.Namespace `json:"listed_arrows"`
}
