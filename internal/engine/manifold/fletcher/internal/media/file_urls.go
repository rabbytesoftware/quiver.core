package media

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type fileURLs struct {
	raw string
}

func fileURLsOf(
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) fileURLs {
	raw, _ := host.RawFileURL(ns, ref, readme.FilePlaceholder)
	return fileURLs{raw: raw}
}

func (u fileURLs) pinned(
	path string,
) (string, bool) {
	if !strings.Contains(u.raw, readme.FilePlaceholder) {
		return "", false
	}
	return strings.ReplaceAll(u.raw, readme.FilePlaceholder, path), true
}
