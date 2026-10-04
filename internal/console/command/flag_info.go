package command

// FlagInfo describes one flag of a console command.
type FlagInfo struct {
	Name      string
	Shorthand string
	Usage     string
	Takes     bool
}
