package requirements

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SRVInterface defines the System Requirements Validation interface
// This interface provides comprehensive system compatibility and capability verification
type SRVInterface interface {
	Validate(
		ctx context.Context,
		requirements *domain.Requirement,
	) (bool, error)

	ValidateOS(
		ctx context.Context,
		recommendedOS domain.OS,
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
