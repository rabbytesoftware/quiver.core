package models

// RawStep is a unified YAML step with a type discriminator and optional
// OS-specific overrides. Domain step types are separate structs that don't
// support multi-platform override maps, so this intermediate form is required.
type RawStep struct {
	Type          string                     `yaml:"type"`
	Command       string                     `yaml:"command"`
	URL           string                     `yaml:"url"`
	To            string                     `yaml:"to"`
	Signal        string                     `yaml:"signal"`
	Title         string                     `yaml:"title"`
	Timeout       string                     `yaml:"timeout"`
	ExitOnFailure *bool                      `yaml:"exit_on_failure"`
	Overrides     map[string]RawStepOverride `yaml:"overrides"`
}
