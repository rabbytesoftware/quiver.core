package dto

import (
	"encoding/json"

	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

// ConfigDTO is the daemon configuration seen three ways at once.
//
// Running is what the process is using, Configured is what the next start will
// use, and Defaults is what ships in the binary. RestartRequired names the
// settings where Running and Configured disagree.
//
// Running carries no api section: the --host flag can override the configured
// host at start, so the daemon cannot report a bind address from configuration
// alone.
type ConfigDTO struct {
	Running         runningConfig        `json:"running"`
	Configured      usecases.Config      `json:"configured"`
	Defaults        usecases.Config      `json:"defaults"`
	RestartRequired []string             `json:"restart_required"`
	Corrected       []ConfigRejectionDTO `json:"corrected"`
}

// runningConfig is the configuration in force, minus the api section.
type runningConfig struct {
	usecases.Config
}

// MarshalJSON drops the api section rather than reporting a bind address the
// daemon cannot vouch for. It works off the encoded form so a new setting
// needs no code here.
func (r runningConfig) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(r.Config)
	if err != nil {
		return nil, err
	}

	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, err
	}

	delete(sections, "api")

	return json.Marshal(sections)
}

// ConfigDTOFrom maps a usecase configuration view onto the wire shape.
func ConfigDTOFrom(
	view usecases.ConfigView,
) ConfigDTO {
	restart := view.RestartRequired
	if restart == nil {
		restart = []string{}
	}

	corrected := make([]ConfigRejectionDTO, 0, len(view.Corrected))
	for _, fe := range view.Corrected {
		corrected = append(corrected, ConfigRejectionDTO{Key: fe.Key, Message: fe.Message})
	}

	return ConfigDTO{
		Running:         runningConfig{Config: view.Running},
		Configured:      view.Configured,
		Defaults:        view.Defaults,
		RestartRequired: restart,
		Corrected:       corrected,
	}
}
