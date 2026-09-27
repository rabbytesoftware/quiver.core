package shelf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func workdirOwner(
	nsDir string,
	target string,
) domain.Namespace {
	rel, err := filepath.Rel(nsDir, target)
	if err != nil || !relInside(rel) {
		return ""
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		bare, _, found := strings.Cut(part, "@")
		if !found {
			continue
		}
		return domain.Namespace(strings.Join(append(parts[:i:i], bare), "/"))
	}
	return ""
}

func markerOwner(
	content string,
	prefix string,
) (domain.Namespace, bool) {
	for _, line := range strings.Split(content, "\n") {
		value, found := strings.CutPrefix(strings.TrimRight(line, "\r"), prefix)
		if found {
			return domain.Namespace(strings.TrimSpace(value)), true
		}
	}
	return "", false
}

func fileHolder(
	path string,
	prefix string,
) (holder, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return holder{}, nil
	}
	if err != nil {
		return holder{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return holder{exists: true}, nil
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return holder{}, fmt.Errorf("read %s: %w", path, err)
	}

	owner, _ := markerOwner(string(data), prefix)
	return holder{exists: true, namespace: owner}, nil
}

func removeMarked(
	dir string,
	prefix string,
	suffix string,
	marker string,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}

	var errs []error
	for _, e := range entries {
		loc := filepath.Join(dir, e.Name())
		if keep[loc] || !e.Type().IsRegular() || !hasPrefixSuffix(e.Name(), prefix, suffix) {
			continue
		}
		h, err := fileHolder(loc, marker)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if h.namespace != bare {
			continue
		}
		if err := os.Remove(loc); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", loc, err))
		}
	}
	return errors.Join(errs...)
}

func hasPrefixSuffix(
	name string,
	prefix string,
	suffix string,
) bool {
	return len(name) > len(prefix)+len(suffix) && name[:len(prefix)] == prefix && hasSuffixFold(name, suffix)
}
