package forge

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

type target struct {
	Lifecycle lifecycle `yaml:"lifecycle"`
	Expose    expose    `yaml:"expose"`
}

func newTarget(
	name string,
	file string,
	platform domain.OS,
	pick picker.Pick,
) target {
	binary := binaryPath(name, platform.IsWindows())
	return target{
		Lifecycle: lifecycle{Install: installSteps(name, executableName(name, platform.IsWindows()), file, pick)},
		Expose:    newExpose(name, binary, platform, pick),
	}
}

type lifecycle struct {
	Install []step `yaml:"install"`
}

const (
	installPath  = "${INSTALL_PATH}"
	stepTimeout  = "15m"
	stepFetch    = "fetch"
	stepPortable = "portable"
	windowsExt   = ".exe"
	downloadExt  = ".download"
	msiExt       = ".msi"
)

type step struct {
	Type     string `yaml:"type"`
	Title    string `yaml:"title,omitempty"`
	URL      string `yaml:"url,omitempty"`
	From     string `yaml:"from,omitempty"`
	To       string `yaml:"to,omitempty"`
	Name     string `yaml:"name,omitempty"`
	Checksum string `yaml:"checksum,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

func installSteps(
	name string,
	executable string,
	file string,
	pick picker.Pick,
) []step {
	download := installPath + "/." + name + downloadExt + downloadSuffix(pick.Format)
	fetch := step{
		Type:     stepFetch,
		Title:    "Download " + file,
		URL:      pick.Asset.URL,
		To:       download,
		Checksum: pick.Asset.Digest,
		Timeout:  stepTimeout,
	}
	portable := step{
		Type:    stepPortable,
		Title:   "Install " + file,
		From:    download,
		To:      installDir(name),
		Name:    executable,
		Timeout: stepTimeout,
	}
	return []step{fetch, portable}
}

func downloadSuffix(
	format picker.Format,
) string {
	if format == picker.FormatMSI {
		return msiExt
	}
	return ""
}

func installDir(
	name string,
) string {
	return installPath + "/" + name
}

func executableName(
	name string,
	windows bool,
) string {
	if windows {
		return name + windowsExt
	}
	return name
}

func binaryPath(
	name string,
	windows bool,
) string {
	return installDir(name) + "/" + executableName(name, windows)
}
