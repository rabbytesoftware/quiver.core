package arrow

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type ArrowListDTO struct {
	Namespace   domain.Namespace  `json:"namespace"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	State       domain.ArrowState `json:"state"`
	Tags        []string          `json:"tags"`
}

type ArrowDetailDTO struct {
	Namespace           domain.Namespace            `json:"namespace"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description"`
	Tags                []string                    `json:"tags"`
	Variables           []domain.Variable           `json:"variables"`
	Targets             map[domain.OS]domain.Target `json:"targets"`
	InstalledAt         time.Time                   `json:"installed_at"`
	InstalledRef        string                      `json:"installed_ref"`
	InstalledConstraint string                      `json:"installed_constraint"`
	UserInstalled       bool                        `json:"user_installed"`
	State               domain.ArrowState           `json:"state"`
	ActiveRun           *domainRuntime.Execution    `json:"active_run,omitempty"`
	LastReturn          *domainRuntime.Return       `json:"last_return,omitempty"`
}

// ValidationResult is returned by ValidateManifest.
// Valid is true when the manifest passes all assembler rules.
// On failure, Errors contains one entry per violated rule.
// SupportedPlatforms lists the OS values present in the manifest.
// UnsupportedPlatforms lists the OS values absent from the manifest.
type ValidationResult struct {
	Valid                bool
	Errors               []ValidationError
	SupportedPlatforms   []domain.OS
	UnsupportedPlatforms []domain.OS
}

// ValidationError is a single structured validation failure.
type ValidationError struct {
	Field   string
	Rule    string
	Message string
}
