package models

// RawStepOverride carries OS-specific field replacements for a run step.
type RawStepOverride struct {
	Command       string `yaml:"command"`
	Title         string `yaml:"title"`
	Timeout       string `yaml:"timeout"`
	ExitOnFailure *bool  `yaml:"exit_on_failure"`
}
