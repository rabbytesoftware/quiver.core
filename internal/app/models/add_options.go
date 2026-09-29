package models

// AddOptions carries optional install-time preferences for adding an arrow.
// Mirrors UpdateOptions: a zero value means "no preference," which today
// means the default (stable) channel.
type AddOptions struct {
	Channel string
}
