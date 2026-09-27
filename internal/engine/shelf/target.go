package shelf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func expandPath(
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

func insideDir(
	dir string,
	path string,
) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && relInside(rel)
}

func relInside(
	rel string,
) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func safeName(
	name string,
) bool {
	if name == "" || name == "." || name == ".." || strings.Contains(name, reservedInfix) {
		return false
	}
	return !unsafePath(name, `/\:*?"<>|`)
}

func contained(
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
	if !insideDir(root, filepath.Join(parent, filepath.Base(target))) {
		return reasonOutsideWorkdir, nil
	}

	resolved, err := filepath.EvalSymlinks(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}
	if !insideDir(root, resolved) {
		return reasonOutsideWorkdir, nil
	}
	return "", nil
}

func resolvesInside(
	workdir string,
	target string,
) bool {
	_, ok := resolveInside(workdir, target)
	return ok
}

func resolveInside(
	workdir string,
	target string,
) (string, bool) {
	root, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil || !insideDir(root, resolved) {
		return "", false
	}
	return resolved, true
}

func unsafePath(
	path string,
	reserved string,
) bool {
	return strings.ContainsAny(path, reserved) || strings.IndexFunc(path, unicode.IsControl) >= 0
}

func requireTarget(
	target string,
	wantDir bool,
) (string, error) {
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return reasonNotFound, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", target, err)
	}
	if info.IsDir() != wantDir {
		return reasonWrongType, nil
	}
	return "", nil
}
