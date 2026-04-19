package arrow

import (
	"time"

	depspkg "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps"
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

// UpdateOptions controls which side effects arrowService.Update applies.
// Zero value = preview mode (updates manifest metadata only).
type UpdateOptions struct {
	InstallAdded     bool
	UninstallOrphans bool
	UpgradeRef       bool
}

// UpdateResult is the response from arrowService.Update.
// Always fully populated regardless of which options were set.
type UpdateResult struct {
	AddedDeps           []domain.Namespace
	RemovedFromManifest []domain.Namespace
	SafeToUninstall     []domain.Namespace
	ConstrainedDeps     []depspkg.ConstraintChange
	NewRef              string
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
