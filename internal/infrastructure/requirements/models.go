package requirements

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/system"
)

// SRVInterface defines the System Requirements Validation interface
// This interface provides comprehensive system compatibility and capability verification
type SRVInterface interface {
	Validate(
		ctx context.Context,
		requirements *requirement.Requirement,
	) (bool, error)

	ValidateOS(
		ctx context.Context,
		recommendedOS system.OS,
	) (bool, error)
	ValidateArch(
		ctx context.Context,
		recommendedArch string,
	) (bool, error)

	ValidateCPU(
		ctx context.Context,
		recommendedCPU int,
	) (bool, error)
	ValidateMemory(
		ctx context.Context,
		recommendedMemory int,
	) (bool, error)
	ValidateDisk(
		ctx context.Context,
		recommendedDisk int,
	) (bool, error)
}
