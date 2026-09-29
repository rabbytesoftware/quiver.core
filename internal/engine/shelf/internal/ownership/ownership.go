package ownership

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
)

type Holder struct {
	Exists    bool
	Namespace domain.Namespace
}

func (h Holder) Refusal(
	bare domain.Namespace,
) string {
	if !h.Exists || h.Namespace == bare {
		return ""
	}
	if h.Namespace == "" {
		return models.ReasonUnmanaged
	}
	return fmt.Sprintf("%s %s", models.ReasonForeignOwner, h.Namespace)
}

func WorkdirOwner(
	nsDir string,
	target string,
) domain.Namespace {
	rel, err := filepath.Rel(nsDir, target)
	if err != nil || !fsguard.RelInside(rel) {
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

func FileHolder(
	path string,
	prefix string,
) (Holder, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Holder{}, nil
	}
	if err != nil {
		return Holder{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Holder{Exists: true}, nil
	}

	data, err := os.ReadFile(path) // #nosec G304 -- path is an entry Quiver is inspecting for its ownership marker
	if err != nil {
		return Holder{}, fmt.Errorf("read %s: %w", path, err)
	}

	owner, _ := markerOwner(string(data), prefix)
	return Holder{Exists: true, Namespace: owner}, nil
}

func RemoveMarked(
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
		h, err := FileHolder(loc, marker)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if h.Namespace != bare {
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
	return len(name) > len(prefix)+len(suffix) && name[:len(prefix)] == prefix && fsguard.HasSuffixFold(name, suffix)
}
