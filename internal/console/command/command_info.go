package command

// CommandInfo describes one command the console may run.
type CommandInfo struct {
	Path    []string
	Short   string
	Usage   string
	Aliases []string
	Flags   []FlagInfo
}
