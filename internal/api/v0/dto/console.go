package dto

type ConsoleCommandDTO struct {
	Path  []string `json:"path" yaml:"path"`
	Short string   `json:"short" yaml:"short"`
	Usage string   `json:"usage" yaml:"usage"`
}

type ConsoleCommandsDTO struct {
	Commands []ConsoleCommandDTO `json:"commands" yaml:"commands"`
}

type ConsoleExecRequestDTO struct {
	Line string `json:"line" yaml:"line"`
}
