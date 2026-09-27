package shelf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	pathMarker       = "# quiver path"
	userPathLocation = `HKCU\Environment\Path`
)

func (s *shelf) pathStatus() (PathStatus, error) {
	l, err := s.layout()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	status := PathStatus{BinDir: l.bin, OnPath: s.onPath(l.bin)}
	if s.goos == goosWindows {
		return s.windowsStatus(status)
	}
	return s.rcStatus(status, l.userHome)
}

func (s *shelf) setupPath() (PathStatus, error) {
	l, err := s.layout()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}

	if err := s.configurePath(l); err != nil {
		return PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}
	return s.pathStatus()
}

func (s *shelf) configurePath(
	l layout,
) error {
	if s.goos == goosWindows {
		return s.setupWindowsPath(l.bin)
	}

	for _, file := range s.rcFiles(l.userHome) {
		if err := ensureBlock(file, l.bin); err != nil {
			return err
		}
	}
	return nil
}

func (s *shelf) onPath(
	bin string,
) bool {
	sep := ":"
	if s.goos == goosWindows {
		sep = ";"
	}
	return pathListContains(s.env("PATH"), bin, sep, s.goos == goosWindows)
}

func (s *shelf) rcStatus(
	status PathStatus,
	userHome string,
) (PathStatus, error) {
	status.Files = s.rcFiles(userHome)
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

func (s *shelf) rcFiles(
	userHome string,
) []string {
	shell := filepath.Base(s.env("SHELL"))
	if shell == "." && s.goos == goosDarwin {
		shell = "zsh"
	}

	switch shell {
	case "zsh":
		return []string{filepath.Join(userHome, ".zshrc")}
	case "bash":
		return s.bashFiles(userHome)
	case "fish":
		return []string{filepath.Join(userHome, ".config", "fish", "conf.d", "quiver.fish")}
	}
	return []string{filepath.Join(userHome, ".profile")}
}

func (s *shelf) bashFiles(
	userHome string,
) []string {
	files := []string{filepath.Join(userHome, ".bashrc")}
	if s.goos == goosDarwin {
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
	data, err := os.ReadFile(file) //nolint:gosec
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
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec
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

func (s *shelf) windowsStatus(
	status PathStatus,
) (PathStatus, error) {
	current, err := s.userPath.read()
	if err != nil {
		return PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	status.Configured = pathListContains(current, status.BinDir, ";", true)
	status.Files = []string{userPathLocation}
	return status, nil
}

func (s *shelf) setupWindowsPath(
	bin string,
) error {
	current, err := s.userPath.read()
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
	return s.userPath.write(next)
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
