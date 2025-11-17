package arrow

import (
	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/models/port"
	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/runtime"
	"github.com/rabbytesoftware/quiver/internal/models/system"
	"github.com/rabbytesoftware/quiver/internal/models/variable"
)

type Arrow struct {
	ID            uuid.UUID      `json:"id"`
	Namespace     ArrowNamespace `json:"namespace"`
	ArrowVersion  []string       `json:"arrow_version" gorm:"serializer:json"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Version       string         `json:"version"`
	License       string         `json:"license"`
	Maintainers   []string       `json:"maintainers" gorm:"serializer:json"`
	Credits       []string       `json:"credits" gorm:"serializer:json"`
	URL           system.URL     `json:"url"`
	Documentation string         `json:"documentation"`

	Requirements requirement.Requirement `json:"requirements" gorm:"serializer:json"`
	Dependencies []ArrowNamespace        `json:"dependencies" gorm:"serializer:json"`

	Netbridge []port.PortRule     `json:"netbridge" gorm:"serializer:json"`
	Variables []variable.Variable `json:"variables" gorm:"serializer:json"`

	Methods []runtime.Method `json:"methods" gorm:"serializer:json"`
}
