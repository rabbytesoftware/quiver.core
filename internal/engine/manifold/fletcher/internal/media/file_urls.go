package media

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/readme"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const anyRef = "{ref}"

type fileURLs struct {
	raw     string
	rawAny  string
	blobAny string
}

func fileURLsOf(
	host hosts.Host,
	ns domain.Namespace,
	ref string,
) fileURLs {
	raw, _ := host.RawFileURL(ns, ref, readme.FilePlaceholder)
	rawAny, _ := host.RawFileURL(ns, anyRef, readme.FilePlaceholder)
	blobAny, _ := host.BlobFileURL(ns, anyRef, readme.FilePlaceholder)
	return fileURLs{
		raw:     raw,
		rawAny:  rawAny,
		blobAny: blobAny,
	}
}

func (u fileURLs) pinned(
	path string,
) (string, bool) {
	if !strings.Contains(u.raw, readme.FilePlaceholder) {
		return "", false
	}
	return strings.ReplaceAll(u.raw, readme.FilePlaceholder, path), true
}

func (u fileURLs) resolve(
	src string,
) (string, bool) {
	if readme.IsRelativePath(src) {
		return readme.CleanRelativePath(src), true
	}
	if path, ok := pathUnder(src, u.rawAny); ok {
		return path, true
	}
	return pathUnder(src, u.blobAny)
}

func pathUnder(
	src string,
	template string,
) (string, bool) {
	prefix, rest, ok := strings.Cut(template, anyRef)
	if !ok || prefix == "" {
		return "", false
	}
	separator, _, ok := strings.Cut(rest, readme.FilePlaceholder)
	if !ok || separator == "" {
		return "", false
	}
	remainder, ok := strings.CutPrefix(src, prefix)
	if !ok {
		return "", false
	}
	_, path, ok := strings.Cut(remainder, separator)
	return path, ok
}
