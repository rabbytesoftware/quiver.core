package domain

import "fmt"

const (
	MinCPUCores = 1
	MinMemoryGB = 1
	MinDiskGB   = 1
)

type Requirement struct {
	CpuCores int `yaml:"cpu_cores" json:"cpu_cores"`
	MemoryGB int `yaml:"ram_gb"    json:"memory_gb"`
	DiskGB   int `yaml:"disk_gb"   json:"disk_gb"`
}

func (r *Requirement) IsValid() bool {
	return r.CpuCores >= MinCPUCores &&
		r.MemoryGB >= MinMemoryGB &&
		r.DiskGB >= MinDiskGB
}

func (r *Requirement) Validate() error {
	if r.CpuCores < MinCPUCores {
		return fmt.Errorf("cpu_cores must be >= %d, got %d", MinCPUCores, r.CpuCores)
	}
	if r.MemoryGB < MinMemoryGB {
		return fmt.Errorf("memory_gb must be >= %d, got %d", MinMemoryGB, r.MemoryGB)
	}
	if r.DiskGB < MinDiskGB {
		return fmt.Errorf("disk_gb must be >= %d, got %d", MinDiskGB, r.DiskGB)
	}
	return nil
}
