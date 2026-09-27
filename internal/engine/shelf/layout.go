package shelf

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

type layout struct {
	bin        string
	namespaces string
	userHome   string
	apps       []string
}

func (s *shelf) layout() (layout, error) {
	bin, err := s.binDir()
	if err != nil {
		return layout{}, err
	}

	namespaces, err := s.namespacesDir()
	if err != nil {
		return layout{}, err
	}

	userHome, err := s.userHome()
	if err != nil {
		return layout{}, err
	}

	return layout{
		bin:        bin,
		namespaces: namespaces,
		userHome:   userHome,
		apps:       s.applicationsDirs(userHome),
	}, nil
}

func (s *shelf) binDir() (string, error) {
	if s.homeDir != "" {
		return paths.BinAt(s.homeDir)
	}
	return paths.Bin()
}

func (s *shelf) namespacesDir() (string, error) {
	if s.homeDir != "" {
		return paths.NamespacesAt(s.homeDir)
	}
	return paths.Namespaces()
}

func (s *shelf) userHome() (string, error) {
	if s.userHomeDir != "" {
		return s.userHomeDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return home, nil
}

func (s *shelf) applicationsDirs(
	userHome string,
) []string {
	if s.appsDirs != nil {
		return s.appsDirs
	}
	return []string{"/Applications", filepath.Join(userHome, "Applications")}
}
