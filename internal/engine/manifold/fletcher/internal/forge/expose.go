package forge

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

type exposeEntry struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

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

const (
	exposeNameLimit    = 60
	exposeNameFallback = "app"
)

func exposeName(
	repo string,
) string {
	var b strings.Builder
	for _, r := range repo {
		b.WriteRune(exposeRune(r))
	}
	name := strings.TrimLeft(b.String(), "._+-")
	if domain.IsWindowsReservedName(name) {
		name = exposeNameFallback + "-" + name
	}
	if len(name) > exposeNameLimit {
		name = name[:exposeNameLimit]
	}
	name = strings.TrimRight(name, ".")
	if name == "" {
		return exposeNameFallback
	}
	return name
}

func exposeRune(
	r rune,
) rune {
	if isAlnum(r) || strings.ContainsRune("._+-", r) {
		return r
	}
	return '-'
}

func isAlnum(
	r rune,
) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}
