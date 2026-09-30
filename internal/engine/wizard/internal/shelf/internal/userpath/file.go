package userpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type fileUserPath struct {
	path string
}

func NewFile(
	path string,
) UserPath {
	return &fileUserPath{path: path}
}

func (f *fileUserPath) Read() (string, error) {
	data, err := os.ReadFile(f.path) // #nosec G304 -- path is the sandbox user PATH file the shelf was built with
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", f.path, err)
	}
	return string(data), nil
}

func (f *fileUserPath) Write(
	value string,
) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(f.path), err)
	}
	if err := os.WriteFile(f.path, []byte(value), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", f.path, err)
	}
	return nil
}

func (*fileUserPath) Broadcast() error {
	return nil
}

func (f *fileUserPath) Location() string {
	return f.path
}
