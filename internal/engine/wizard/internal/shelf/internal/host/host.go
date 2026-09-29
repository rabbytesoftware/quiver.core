package host

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

type Host struct {
	GOARCH      string
	HomeDir     string
	UserHomeDir string
	SandboxHome string
	AppsDirs    []string
	Env         func(string) string
	Commander   Commander
	Rename      func(string, string) error
	Chmod       func(string, os.FileMode) error
}

func New() Host {
	return Host{
		GOARCH:    runtime.GOARCH,
		Env:       os.Getenv,
		Commander: NewCommander(),
		Rename:    os.Rename,
		Chmod:     os.Chmod,
	}
}

func (h Host) Layout() (models.Layout, error) {
	bin, err := h.binDir()
	if err != nil {
		return models.Layout{}, err
	}

	namespaces, err := h.namespacesDir()
	if err != nil {
		return models.Layout{}, err
	}

	userHome, err := h.userHome()
	if err != nil {
		return models.Layout{}, err
	}

	return models.Layout{
		Bin:        bin,
		Namespaces: namespaces,
		UserHome:   userHome,
		AppData:    h.appData(userHome),
		Apps:       h.applicationsDirs(userHome),
	}, nil
}

func (h Host) appData(
	userHome string,
) string {
	if h.SandboxHome != "" {
		return filepath.Join(h.SandboxHome, "AppData", "Roaming")
	}
	if v := h.Env("APPDATA"); v != "" {
		return v
	}
	return filepath.Join(userHome, "AppData", "Roaming")
}

func (h Host) binDir() (string, error) {
	if h.HomeDir != "" {
		return paths.BinAt(h.HomeDir)
	}
	return paths.Bin()
}

func (h Host) namespacesDir() (string, error) {
	if h.HomeDir != "" {
		return paths.NamespacesAt(h.HomeDir)
	}
	return paths.Namespaces()
}

func (h Host) userHome() (string, error) {
	if h.UserHomeDir != "" {
		return h.UserHomeDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home: %w", err)
	}
	return home, nil
}

func (h Host) applicationsDirs(
	userHome string,
) []string {
	if h.AppsDirs != nil {
		return h.AppsDirs
	}
	return []string{"/Applications", filepath.Join(userHome, "Applications")}
}
