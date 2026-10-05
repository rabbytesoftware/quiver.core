package dto

// ConsoleCommandDTO describes one command the console may run.
//
// Path is the words that name it after the leading quiver, for example
// ["arrow", "add"]. Short is its one-line description and Usage is its usage
// line, such as "install <namespace>", both taken from the command itself.
type ConsoleCommandDTO struct {
	Path  []string `json:"path" yaml:"path"`
	Short string   `json:"short" yaml:"short"`
	Usage string   `json:"usage" yaml:"usage"`
}
