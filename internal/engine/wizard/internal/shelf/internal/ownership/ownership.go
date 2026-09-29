package ownership

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

type Holder struct {
	Exists    bool
	Namespace domain.Namespace
	Target    string
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

type Claim func(h Holder) bool

func NamespaceClaim(
	bare domain.Namespace,
) Claim {
	return func(h Holder) bool { return h.Namespace == bare }
}

// WorkdirClaim takes the entries pointing into workdir, plus those of the same
// namespace pointing at nothing: a renamed or deleted workdir leaves entries
// nothing else will ever claim, while an entry pointing into a sibling ref's
// live workdir belongs to that ref.
func WorkdirClaim(
	bare domain.Namespace,
	workdir string,
) Claim {
	return func(h Holder) bool {
		if h.Namespace != bare {
			return false
		}
		return workfs.Inside(workdir, h.Target) || !present(h.Target)
	}
}

func present(
	path string,
) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func WorkdirOwner(
	nsDir string,
	target string,
) domain.Namespace {
	rel, err := filepath.Rel(nsDir, target)
	if err != nil || !workfs.RelInside(rel) {
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

type Marker struct {
	Owner   string
	Workdir string
}

func (m Marker) Lines(
	bare domain.Namespace,
	workdir string,
	eol string,
) string {
	return m.Owner + string(bare) + eol + m.Workdir + workdir + eol
}

func (m Marker) holder(
	content string,
) Holder {
	owner, _ := markerValue(content, m.Owner)
	workdir, _ := markerValue(content, m.Workdir)
	return Holder{Exists: true, Namespace: domain.Namespace(owner), Target: workdir}
}

func markerValue(
	content string,
	prefix string,
) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		value, found := strings.CutPrefix(strings.TrimRight(line, "\r"), prefix)
		if found {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func FileHolder(
	path string,
	marker Marker,
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

	return marker.holder(string(data)), nil
}

func RemoveMarked(
	dir string,
	prefix string,
	suffix string,
	marker Marker,
	claim Claim,
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
		if !claim(h) {
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
