package pathenv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

type PathStatus struct {
	BinDir     string
	OnPath     bool
	Configured bool
	Files      []string
}

type UserPath interface {
	Read() (string, error)
	Write(
		value string,
	) error
}

type Manager interface {
	Status() (PathStatus, error)
	Setup() (PathStatus, error)
}

type manager struct {
	host     platform.Host
	userPath UserPath
}

func New(
	host platform.Host,
	userPath UserPath,
) Manager {
	return &manager{host: host, userPath: userPath}
}

const (
	pathMarker       = "# quiver path"
	userPathLocation = `HKCU\Environment\Path`
)

func (m *manager) Status() (PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	status := PathStatus{BinDir: l.Bin, OnPath: m.onPath(l.Bin)}
	if m.host.GOOS == platform.GOOSWindows {
		return m.windowsStatus(status)
	}
	return m.rcStatus(status, l.UserHome)
}

func (m *manager) Setup() (PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}

	if err := m.configurePath(l); err != nil {
		return PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}
	return m.Status()
}

func (m *manager) configurePath(
	l platform.Layout,
) error {
	if m.host.GOOS == platform.GOOSWindows {
		return m.setupWindowsPath(l.Bin)
	}

	for _, file := range m.rcFiles(l.UserHome) {
		if err := ensureBlock(file, l.Bin); err != nil {
			return err
		}
	}
	return nil
}

func (m *manager) onPath(
	bin string,
) bool {
	sep := ":"
	if m.host.GOOS == platform.GOOSWindows {
		sep = ";"
	}
	return pathListContains(m.host.Env("PATH"), bin, sep, m.host.GOOS == platform.GOOSWindows)
}

func (m *manager) rcStatus(
	status PathStatus,
	userHome string,
) (PathStatus, error) {
	status.Files = m.rcFiles(userHome)
	status.Configured = true
	for _, file := range status.Files {
		_, marked, err := readRC(file)
		if err != nil {
			return PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
		}
		status.Configured = status.Configured && marked
	}
	return status, nil
}

func (m *manager) rcFiles(
	userHome string,
) []string {
	shell := filepath.Base(m.host.Env("SHELL"))
	if shell == "." && m.host.GOOS == platform.GOOSDarwin {
		shell = "zsh"
	}

	switch shell {
	case "zsh":
		return []string{filepath.Join(userHome, ".zshrc")}
	case "bash":
		return m.bashFiles(userHome)
	case "fish":
		return []string{filepath.Join(userHome, ".config", "fish", "conf.d", "quiver.fish")}
	}
	return []string{filepath.Join(userHome, ".profile")}
}

func (m *manager) bashFiles(
	userHome string,
) []string {
	files := []string{filepath.Join(userHome, ".bashrc")}
	if m.host.GOOS == platform.GOOSDarwin {
		files = append(files, filepath.Join(userHome, ".bash_profile"))
	}
	return files
}

func rcBlock(
	file string,
	bin string,
) string {
	if strings.HasSuffix(file, ".fish") {
		return pathMarker + "\ncontains -- \"" + bin + "\" $PATH; or set -gx PATH $PATH \"" + bin + "\"\n"
	}
	return pathMarker + "\nexport PATH=\"$PATH:" + bin + "\"\n"
}

func readRC(
	file string,
) (string, bool, error) {
	data, err := os.ReadFile(file) // #nosec G304 -- path is a shell rc file Quiver manages in the user's home
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", file, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pathMarker {
			return string(data), true, nil
		}
	}
	return string(data), false, nil
}

func ensureBlock(
	file string,
	bin string,
) error {
	content, marked, err := readRC(file)
	if err != nil || marked {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(file), err)
	}
	return appendBlock(file, separator(content)+rcBlock(file, bin))
}

func separator(
	existing string,
) string {
	if existing == "" {
		return ""
	}
	if strings.HasSuffix(existing, "\n") {
		return "\n"
	}
	return "\n\n"
}

func appendBlock(
	file string,
	block string,
) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- path is a shell rc file Quiver manages in the user's home
	if err != nil {
		return fmt.Errorf("open %s: %w", file, err)
	}
	if _, err := f.WriteString(block); err != nil {
		_ = f.Close()
		return fmt.Errorf("append %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", file, err)
	}
	return nil
}

func (m *manager) windowsStatus(
	status PathStatus,
) (PathStatus, error) {
	current, err := m.userPath.Read()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	status.Configured = pathListContains(current, status.BinDir, ";", true)
	status.Files = []string{userPathLocation}
	return status, nil
}

func (m *manager) setupWindowsPath(
	bin string,
) error {
	current, err := m.userPath.Read()
	if err != nil {
		return err
	}
	if pathListContains(current, bin, ";", true) {
		return nil
	}

	next := bin
	if trimmed := strings.TrimRight(current, ";"); trimmed != "" {
		next = trimmed + ";" + bin
	}
	return m.userPath.Write(next)
}

func pathListContains(
	list string,
	dir string,
	sep string,
	fold bool,
) bool {
	want := filepath.Clean(dir)
	for _, entry := range strings.Split(list, sep) {
		if entry == "" {
			continue
		}
		got := filepath.Clean(entry)
		if got == want || (fold && strings.EqualFold(got, want)) {
			return true
		}
	}
	return false
}
