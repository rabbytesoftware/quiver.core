package shelf

import (
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type applyRequest struct {
	layout  layout
	bare    domain.Namespace
	workdir string
	media   domain.ArrowMedia
	moved   map[string]string
}

func (r applyRequest) relocate(
	target string,
) string {
	for src, dest := range r.moved {
		rel, err := filepath.Rel(src, target)
		if err != nil || !relInside(rel) {
			continue
		}
		return filepath.Join(dest, rel)
	}
	return target
}
