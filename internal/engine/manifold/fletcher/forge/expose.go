package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

type expose struct {
	CLI     []exposeEntry `yaml:"cli,omitempty"`
	Desktop []exposeEntry `yaml:"desktop,omitempty"`
}

func newExpose(
	name string,
	binary string,
	platform domain.OS,
	pick picker.Pick,
) expose {
	entries := []exposeEntry{{Name: name, Path: domain.ExposeAuto}}
	if pick.Format == picker.FormatBinary {
		return expose{CLI: []exposeEntry{{Name: name, Path: binary}}}
	}
	if pick.Format == picker.FormatDMG || pick.Format == picker.FormatAppImage {
		return expose{Desktop: entries}
	}
	if pick.Format == picker.FormatArchive && (platform.IsDarwin() || pick.GUI) {
		return expose{CLI: entries, Desktop: entries}
	}
	return expose{CLI: entries}
}
