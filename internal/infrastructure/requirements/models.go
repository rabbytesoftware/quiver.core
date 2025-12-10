package requirements

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

// SRVInterface defines the System Requirements Validation interface
// This interface provides comprehensive system compatibility and capability verification
type SRVInterface interface {
	Validate(
		ctx context.Context,
		requirements *arrow.Requirement,
	) (bool, error)

	ValidateOS(
		ctx context.Context,
		recommendedOS shared.OS,
	) (bool, error)
	ValidateOSVersion(
		ctx context.Context,
		recommendedVersion string,
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
	ValidateNetwork(
		ctx context.Context,
		recommendedNetwork int,
	) (bool, error)
}
