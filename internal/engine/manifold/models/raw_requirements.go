package models

// RawRequirements captures system resource requirements from YAML.
// Explicit yaml tags are required because the YAML keys use snake_case
// (cpu_cores, ram_gb, disk_gb) which don't match Go field name lowercasing.
type RawRequirements struct {
	CpuCores int      `yaml:"cpu_cores"`
	RamGB    int      `yaml:"ram_gb"`
	DiskGB   int      `yaml:"disk_gb"`
	OS       []string `yaml:"os"`
}
