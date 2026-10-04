package command

// FlagInfo describes one flag of a console command.
type FlagInfo struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand"`
	Usage     string `json:"usage"`
	Takes     bool   `json:"takes_value"`
}
