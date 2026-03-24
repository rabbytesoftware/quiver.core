package domain

import "fmt"

const (
	MinCPUCores = 1
	MinMemoryGB = 1
	MinDiskGB   = 1
)

type Requirement struct {
	CpuCores int
	MemoryGB int
	DiskGB   int
	OS       []OS
}

func (r *Requirement) IsValid() bool {
	if r.CpuCores < MinCPUCores || r.MemoryGB < MinMemoryGB || r.DiskGB < MinDiskGB {
		return false
	}
	for _, o := range r.OS {
		if !o.IsValid() {
			return false
		}
	}
	return true
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
	for _, o := range r.OS {
		if !o.IsValid() {
			return fmt.Errorf("invalid OS: %s", o)
		}
	}
	return nil
}
