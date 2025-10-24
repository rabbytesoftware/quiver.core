package arrow

import (
	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/runtime"
	"github.com/rabbytesoftware/quiver/internal/models/system"
	"github.com/rabbytesoftware/quiver/internal/models/variable"
)

type Arrow struct {
	ID            uuid.UUID      `json:"id"`
	Namespace     ArrowNamespace `json:"namespace"`
	ArrowVersion  []string       `json:"arrow_version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Version       string         `json:"version"`
	License       string         `json:"license"`
	Maintainers   []string       `json:"maintainers"`
	Credits       []string       `json:"credits"`
	URL           system.URL     `json:"url"`
	Documentation string         `json:"documentation"`

	Requirements requirement.Requirement `json:"requirements"`
	Dependencies []ArrowNamespace        `json:"dependencies"`

	Netbridge []networking.Port         `json:"netbridge"`
	Variables []variable.Variable `json:"variables"`

	Methods []runtime.Method `json:"methods"`
}
