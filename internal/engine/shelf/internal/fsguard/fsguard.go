package fsguard

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
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

func InsideDir(
	dir string,
	path string,
) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && RelInside(rel)
}

func RelInside(
	rel string,
) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	if !InsideDir(root, filepath.Join(parent, filepath.Base(target))) {
		return models.ReasonOutsideWorkdir, nil
	}

	resolved, err := filepath.EvalSymlinks(target)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", target, err)
	}
	if !InsideDir(root, resolved) {
		return models.ReasonOutsideWorkdir, nil
	}
	return "", nil
}

func ResolvesInside(
	workdir string,
	target string,
) bool {
	_, ok := ResolveInside(workdir, target)
	return ok
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
	if err != nil || !InsideDir(root, resolved) {
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

func Relocate(
	req models.ApplyRequest,
	target string,
) string {
	for src, dest := range req.Moved {
		rel, err := filepath.Rel(src, target)
		if err != nil || !RelInside(rel) {
			continue
		}
		return filepath.Join(dest, rel)
	}
	return target
}
