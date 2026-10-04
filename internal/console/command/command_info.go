package command

// CommandInfo describes one command the console may run.
type CommandInfo struct {
	Path    []string   `json:"path"`
	Short   string     `json:"short"`
	Usage   string     `json:"usage"`
	Aliases []string   `json:"aliases"`
	Flags   []FlagInfo `json:"flags"`
}
