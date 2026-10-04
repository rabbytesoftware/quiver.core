package dto

type ConsoleCommandDTO struct {
	Path    []string         `json:"path" yaml:"path"`
	Short   string           `json:"short" yaml:"short"`
	Usage   string           `json:"usage" yaml:"usage"`
	Aliases []string         `json:"aliases" yaml:"aliases"`
	Flags   []ConsoleFlagDTO `json:"flags" yaml:"flags"`
}
