package models

// AddOptions carries optional install-time preferences for adding an arrow.
// Channel names the selector a refless namespace follows; a zero value
// follows the repository's default channel, and a namespace that already
// carries a selector ignores it.
type AddOptions struct {
	Channel string
}
