package fsguard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

func ExpandPath(
	raw string,
	workdir string,
) string {
	expanded := strings.NewReplacer("${INSTALL_PATH}", workdir, "${WORKDIR}", workdir).Replace(raw)
	expanded = filepath.FromSlash(expanded)
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Join(workdir, expanded)
}

func SafeName(
	name string,
) bool {
	if name == "" || name == "." || name == ".." || strings.Contains(name, reservedInfix) {
		return false
	}
	return !UnsafePath(name, `/\:*?"<>|`)
}

func Contained(
	workdir string,
	target string,
) (string, error) {
	root, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", workdir, err)
	}

	parent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", filepath.Dir(target), err)
	}
	if !workfs.Inside(root, filepath.Join(parent, filepath.Base(target))) {
		return models.ReasonOutsideWorkdir, nil
	}

	resolved, err := filepath.EvalSymlinks(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}
	if !workfs.Inside(root, resolved) {
		return models.ReasonOutsideWorkdir, nil
	}
	return "", nil
}

func MarkExecutable(
	workdir string,
	target string,
	chmod func(string, os.FileMode) error,
) error {
	resolved, ok := ResolveInside(workdir, target)
	if !ok {
		return nil
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 != 0 {
		return nil
	}
	if err := chmod(resolved, info.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("mark %s executable: %w", resolved, err)
	}
	return nil
}

func ResolveInside(
	workdir string,
	target string,
) (string, bool) {
	root, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil || !workfs.Inside(root, resolved) {
		return "", false
	}
	return resolved, true
}

func UnsafePath(
	path string,
	reserved string,
) bool {
	return strings.ContainsAny(path, reserved) || strings.IndexFunc(path, unicode.IsControl) >= 0
}

func RequireTarget(
	target string,
	wantDir bool,
) (string, error) {
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return models.ReasonNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", target, err)
	}
	if info.IsDir() != wantDir {
		return models.ReasonWrongType, nil
	}
	return "", nil
}

func HasSuffixFold(
	name string,
	suffix string,
) bool {
	return len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}
