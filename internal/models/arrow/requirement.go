package arrow

import (
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

const (
	MinCPUCores    = 1
	MinMemoryMB    = 1
	MinDiskGB      = 1
	MinNetworkMbps = 1
)

type Requirement struct {
	CpuCores    int       `json:"cpu_cores"`
	Memory      int       `json:"memory"`
	Disk        int       `json:"disk"`
	NetworkMbps int       `json:"network_mbps"`
	OS          shared.OS `json:"os"`
}

func (r *Requirement) IsValid() bool {
	return r.CpuCores > 0 && r.Memory > 0 && r.Disk > 0 && r.NetworkMbps > 0 && r.OS.IsValid()
}

func (r *Requirement) Validate() error {
	if r.CpuCores < MinCPUCores {
		return fmt.Errorf("cpu_cores must be >= %d, got %d", MinCPUCores, r.CpuCores)
	}
	if r.Memory < MinMemoryMB {
		return fmt.Errorf("memory must be >= %d MB, got %d", MinMemoryMB, r.Memory)
	}
	if r.Disk < MinDiskGB {
		return fmt.Errorf("disk must be >= %d GB, got %d", MinDiskGB, r.Disk)
	}
	if r.NetworkMbps < MinNetworkMbps {
		return fmt.Errorf("network_mbps must be >= %d, got %d", MinNetworkMbps, r.NetworkMbps)
	}
	if r.OS != "" && !r.OS.IsValid() {
		return fmt.Errorf("invalid OS: %s", r.OS)
	}
	return nil
}
