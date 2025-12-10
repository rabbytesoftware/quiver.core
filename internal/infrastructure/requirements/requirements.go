package requirements

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// Requirements implements SRVInterface for system requirements validation
type Requirements struct{}

// NewRequirements creates a new Requirements validator
func NewRequirements() SRVInterface {
	return &Requirements{}
}

func (r *Requirements) Validate(
	ctx context.Context,
	requirements *arrow.Requirement,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if requirements == nil {
		return false, fmt.Errorf("requirements cannot be nil")
	}

	if valid, err := r.ValidateOS(ctx, requirements.OS); !valid || err != nil {
		return false, fmt.Errorf("OS validation failed: %w", err)
	}

	if valid, err := r.ValidateCPU(ctx, requirements.CpuCores); !valid || err != nil {
		return false, fmt.Errorf("CPU validation failed: %w", err)
	}

	if valid, err := r.ValidateMemory(ctx, requirements.Memory); !valid || err != nil {
		return false, fmt.Errorf("memory validation failed: %w", err)
	}

	if valid, err := r.ValidateDisk(ctx, requirements.Disk); !valid || err != nil {
		return false, fmt.Errorf("disk validation failed: %w", err)
	}

	return true, nil
}

func (r *Requirements) ValidateOS(
	ctx context.Context,
	recommendedOS shared.OS,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	parts := strings.Split(string(recommendedOS), "/")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid OS format: %s", recommendedOS)
	}

	requiredOS := parts[0]
	requiredArch := parts[1]

	if runtime.GOOS != requiredOS {
		return false, fmt.Errorf("unsupported OS: have %s, need %s", runtime.GOOS, requiredOS)
	}

	if runtime.GOARCH != requiredArch {
		return false, fmt.Errorf("unsupported architecture: have %s, need %s", runtime.GOARCH, requiredArch)
	}

	return true, nil
}

func (r *Requirements) ValidateCPU(
	ctx context.Context,
	recommendedCPU int,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if recommendedCPU <= 0 {
		return false, fmt.Errorf("invalid CPU requirement: %d", recommendedCPU)
	}

	logicalCores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return false, fmt.Errorf("failed to get CPU count: %w", err)
	}

	if logicalCores < recommendedCPU {
		return false, fmt.Errorf("insufficient CPU cores: have %d, need %d", logicalCores, recommendedCPU)
	}

	return true, nil
}

func (r *Requirements) ValidateMemory(
	ctx context.Context,
	recommendedMemory int,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if recommendedMemory <= 0 {
		return false, fmt.Errorf("invalid memory requirement: %d MB", recommendedMemory)
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get memory info: %w", err)
	}

	totalMemoryMB := vm.Total / (1024 * 1024)
	requiredMemoryMB := uint64(recommendedMemory)

	if totalMemoryMB < requiredMemoryMB {
		return false, fmt.Errorf("insufficient memory: have %d MB, need %d MB", totalMemoryMB, requiredMemoryMB)
	}

	return true, nil
}

func (r *Requirements) ValidateDisk(
	ctx context.Context,
	recommendedDisk int,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if recommendedDisk <= 0 {
		return false, fmt.Errorf("invalid disk requirement: %d MB", recommendedDisk)
	}

	usage, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return false, fmt.Errorf("failed to get disk info: %w", err)
	}

	availableDiskMB := usage.Free / (1024 * 1024)
	requiredDiskMB := uint64(recommendedDisk)

	if availableDiskMB < requiredDiskMB {
		return false, fmt.Errorf("insufficient disk space: have %d MB, need %d MB", availableDiskMB, requiredDiskMB)
	}

	return true, nil
}
