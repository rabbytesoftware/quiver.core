package requirements

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/domain"
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
	requirements *domain.Requirement,
) (bool, error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	if requirements == nil {
		return false, fmt.Errorf("requirements cannot be nil")
	}

	if len(requirements.OS) > 0 {
		osMatch := false
		var lastErr error
		for _, os := range requirements.OS {
			valid, err := r.ValidateOS(ctx, os)
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			if valid {
				osMatch = true
				break
			}
			lastErr = err
		}
		if !osMatch {
			return false, fmt.Errorf("OS validation failed: %w", lastErr)
		}
	}

	if valid, err := r.ValidateCPU(ctx, requirements.CpuCores); !valid || err != nil {
		return false, fmt.Errorf("CPU validation failed: %w", err)
	}

	if valid, err := r.ValidateMemory(ctx, requirements.MemoryGB); !valid || err != nil {
		return false, fmt.Errorf("memory validation failed: %w", err)
	}

	if valid, err := r.ValidateDisk(ctx, requirements.DiskGB); !valid || err != nil {
		return false, fmt.Errorf("disk validation failed: %w", err)
	}

	return true, nil
}

func (r *Requirements) ValidateOS(
	ctx context.Context,
	recommendedOS domain.OS,
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
		return false, fmt.Errorf("invalid memory requirement: %d GB", recommendedMemory)
	}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get memory info: %w", err)
	}

	totalMemoryGB := vm.Total / (1024 * 1024 * 1024)
	requiredMemoryGB := uint64(recommendedMemory)

	if totalMemoryGB < requiredMemoryGB {
		return false, fmt.Errorf("insufficient memory: have %d GB, need %d GB", totalMemoryGB, requiredMemoryGB)
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
		return false, fmt.Errorf("invalid disk requirement: %d GB", recommendedDisk)
	}

	usage, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return false, fmt.Errorf("failed to get disk info: %w", err)
	}

	availableDiskGB := usage.Free / (1024 * 1024 * 1024)
	requiredDiskGB := uint64(recommendedDisk)

	if availableDiskGB < requiredDiskGB {
		return false, fmt.Errorf("insufficient disk space: have %d GB, need %d GB", availableDiskGB, requiredDiskGB)
	}

	return true, nil
}
