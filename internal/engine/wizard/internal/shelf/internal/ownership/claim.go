package ownership

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

// ClaimedBy reports whether claim takes h. A workdir claim also takes the
// entries of its namespace pointing at nothing: a renamed or deleted workdir
// leaves entries nothing else will ever claim, while an entry pointing into a
// sibling ref's live workdir belongs to that ref.
func (h Holder) ClaimedBy(
	claim models.Claim,
) bool {
	if h.Namespace != claim.Bare {
		return false
	}
	if claim.Workdir == "" {
		return true
	}
	return workfs.Inside(claim.Workdir, h.Target) || !present(h.Target)
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
