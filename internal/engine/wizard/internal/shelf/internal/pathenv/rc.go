package pathenv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/host"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/userpath"
)

const (
	pathMarker   = "# quiver path"
	unixPathList = ":"
)

type rc struct {
	host         host.Host
	defaultShell string
	bashFiles    []string
}

func NewRC(
	h host.Host,
	defaultShell string,
	bashFiles []string,
) models.PathManager {
	return &rc{host: h, defaultShell: defaultShell, bashFiles: bashFiles}
}

func (m *rc) Status(
	_ context.Context,
) (models.PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
	}

	status := models.PathStatus{
		BinDir:     l.Bin,
		OnPath:     userpath.Contains(m.host.Env("PATH"), l.Bin, unixPathList, false),
		Configured: true,
		Files:      m.files(l.UserHome),
	}
	for _, file := range status.Files {
		_, marked, err := readRC(file)
		if err != nil {
			return models.PathStatus{}, fmt.Errorf("shelf: path status: %w", err)
		}
		status.Configured = status.Configured && marked
	}
	return status, nil
}

func (m *rc) Setup(
	ctx context.Context,
) (models.PathStatus, error) {
	l, err := m.host.Layout()
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
	}

	for _, file := range m.files(l.UserHome) {
		if err := ensureBlock(file, l.Bin); err != nil {
			return models.PathStatus{}, fmt.Errorf("shelf: setup path: %w", err)
		}
	}
	return m.Status(ctx)
}

func (m *rc) files(
	userHome string,
) []string {
	shell := filepath.Base(m.host.Env("SHELL"))
	if shell == "." {
		shell = m.defaultShell
	}

	switch shell {
	case "zsh":
		return []string{filepath.Join(userHome, ".zshrc")}
	case "bash":
		return m.bash(userHome)
	case "fish":
		return []string{filepath.Join(userHome, ".config", "fish", "conf.d", "quiver.fish")}
	}
	return []string{filepath.Join(userHome, ".profile")}
}

func (m *rc) bash(
	userHome string,
) []string {
	files := make([]string, 0, len(m.bashFiles))
	for _, name := range m.bashFiles {
		files = append(files, filepath.Join(userHome, name))
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
