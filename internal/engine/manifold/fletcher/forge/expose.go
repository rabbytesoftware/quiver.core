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
	format picker.Format,
) expose {
	entries := []exposeEntry{{Name: name, Path: domain.ExposeAuto}}
	if format == picker.FormatBinary {
		return expose{CLI: []exposeEntry{{Name: name, Path: binary}}}
	}
	if format == picker.FormatDMG || format == picker.FormatAppImage {
		return expose{Desktop: entries}
	}
	if format == picker.FormatArchive && platform.IsDarwin() {
		return expose{CLI: entries, Desktop: entries}
	}
	return expose{CLI: entries}
}
