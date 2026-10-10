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
	Execute []step `yaml:"execute,omitempty"`
	Stop    []step `yaml:"stop,omitempty"`
}

const (
	installPath    = "${INSTALL_PATH}"
	stepTimeout    = "15m"
	stepFetch      = "fetch"
	stepPortable   = "portable"
	stepRun        = "run"
	stepSignal     = "signal"
	signalGraceful = "graceful"
	stopTimeout    = "10s"
	windowsExt     = ".exe"
	downloadExt    = ".download"
	msiExt         = ".msi"
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
	Command  string `yaml:"command,omitempty"`
	Signal   string `yaml:"signal,omitempty"`
	// ExitOnFailure is a pointer so a step can state false.
	ExitOnFailure *bool `yaml:"exit_on_failure,omitempty"`
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

// startStopSteps run and end the app the install exposed as a desktop entry. The
// run step names the ${ARROW_APP} variable, which the wizard fills with the
// app's executable when the step starts, and fails with a message when the
// install placed none. Stop ends the process and, on Windows, its whole tree.
func startStopSteps(
	name string,
	windows bool,
) ([]step, []step) {
	command := `exec "${ARROW_APP}"`
	if windows {
		command = `"${ARROW_APP}"`
	}
	stayOnFailure := false
	return []step{{Type: stepRun, Title: "Start " + name, Command: command}},
		[]step{{Type: stepSignal, Title: "Stop " + name, Signal: signalGraceful, Timeout: stopTimeout, ExitOnFailure: &stayOnFailure}}
}
