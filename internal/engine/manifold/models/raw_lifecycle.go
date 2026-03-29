package models

// RawLifecycle holds the four platform-managed lifecycle phases of an Arrow,
// each expressed as a slice of RawStep before domain assembly.
type RawLifecycle struct {
	Install   []RawStep `yaml:"install"`
	Execute   []RawStep `yaml:"execute"`
	Stop      []RawStep `yaml:"stop"`
	Uninstall []RawStep `yaml:"uninstall"`
}
