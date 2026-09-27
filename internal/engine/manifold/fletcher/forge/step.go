package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
)

const (
	installPath  = "${INSTALL_PATH}"
	stepTimeout  = "15m"
	stepFetch    = "fetch"
	stepPortable = "portable"
	windowsExt   = ".exe"
)

type step struct {
	Type     string `yaml:"type"`
	Title    string `yaml:"title,omitempty"`
	URL      string `yaml:"url,omitempty"`
	From     string `yaml:"from,omitempty"`
	To       string `yaml:"to,omitempty"`
	Checksum string `yaml:"checksum,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

func installSteps(
	binary string,
	file string,
	pick picker.Pick,
) []step {
	fetch := step{
		Type:     stepFetch,
		Title:    "Download " + file,
		URL:      pick.Asset.URL,
		To:       installPath + "/" + file,
		Checksum: pick.Asset.Digest,
		Timeout:  stepTimeout,
	}
	if pick.Format == picker.FormatBinary {
		fetch.To = binary
		return []step{fetch}
	}
	portable := step{
		Type:    stepPortable,
		Title:   "Install " + file,
		From:    fetch.To,
		To:      installPath,
		Timeout: stepTimeout,
	}
	return []step{fetch, portable}
}

func binaryPath(
	name string,
	windows bool,
) string {
	if windows {
		return installPath + "/" + name + windowsExt
	}
	return installPath + "/" + name
}
