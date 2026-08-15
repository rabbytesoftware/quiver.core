package usecases

import "github.com/rabbytesoftware/quiver.core/internal/core/config"

// ConfigView is the daemon configuration seen three ways at once: what the
// process is running with, what the next start will use, and what ships in the
// binary. A client needs all three to show a current value, offer a reset, and
// say honestly whether a change has taken effect.
type ConfigView struct {
	Running         config.ConfigData
	Configured      config.ConfigData
	Defaults        config.ConfigData
	RestartRequired []string
}

// PatchResult reports which fields a patch persisted and which it refused.
type PatchResult struct {
	Applied  []string
	Rejected []config.FieldError
}
