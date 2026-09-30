package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// windowsReserved holds the characters Windows refuses in a path component,
// plus '%' so an escaped component always decodes back to exactly one ref.
const windowsReserved = `<>:"|?*\%`

// identityReserved adds what keeps two identities apart on disk: '/' so a
// selector is one path segment and never nests inside another identity's
// workdir, and '~', which git forbids in a ref, so it can mark a hashed name.
const identityReserved = windowsReserved + "/~"

const (
	// maxNameLen keeps every name the vault writes well under the 255-byte
	// component limit, and a workdir under a typical Windows home
	// (C:\Users\<name>\.quiver\namespaces\<host>\<user>\) short enough to
	// leave the arrow's own files room under MAX_PATH (260).
	maxNameLen = 96
	// maxComponentLen is the longest name those filesystems hold at all.
	maxComponentLen = 255
	// hashedMarker separates a truncated name from the digest that keeps it
	// unique; git forbids it in a ref, so no plain name ever carries it.
	hashedMarker = "~"
	hashLen      = 32
)

// encodeNS flattens a namespace into a single filename: the bare namespace
// path-escaped, then the selector as one identity segment.
func encodeNS(ns domain.Namespace) string {
	bare, ref, hasRef := strings.Cut(string(ns), "@")
	if !hasRef {
		return capName(url.PathEscape(bare), ns)
	}
	return capName(url.PathEscape(bare)+"@"+encodeRefSegment(ref), ns)
}

// legacyEncodeNS is the manifest cache filename used before identities were
// case-distinct and length-capped; entries it names are still read.
func legacyEncodeNS(ns domain.Namespace) string {
	bare, ref, hasRef := strings.Cut(string(ns), "@")
	if !hasRef {
		return url.PathEscape(bare)
	}
	return url.PathEscape(bare) + "@" + strings.ReplaceAll(url.PathEscape(ref), ":", "%3A")
}

// encodeRefSegment percent-encodes a selector into one path segment that no
// other selector maps to, even on a case-insensitive filesystem: '/', upper
// case letters and non-ASCII bytes are escaped along with everything Windows
// refuses. A plain lower-case tag or branch name keeps its spelling, so the
// layout of an existing plain ref is unchanged.
func encodeRefSegment(ref string) string {
	var b strings.Builder
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if mustEscapeIdentity(c, i == len(ref)-1) {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func mustEscapeIdentity(
	c byte,
	last bool,
) bool {
	if c < 0x20 || c >= 0x7f || (c >= 'A' && c <= 'Z') || strings.IndexByte(identityReserved, c) >= 0 {
		return true
	}
	return last && (c == '.' || c == ' ')
}

// capName keeps name within maxNameLen: a longer one is cut, without
// splitting an escape, and suffixed with a digest of the full identity, so
// two long identities never share a name.
func capName(
	name string,
	ns domain.Namespace,
) string {
	if len(name) <= maxNameLen {
		return name
	}
	cut := maxNameLen - len(hashedMarker) - hashLen
	if i := strings.LastIndexByte(name[:cut], '%'); i >= cut-2 {
		cut = i
	}
	sum := sha256.Sum256([]byte(ns))
	return name[:cut] + hashedMarker + hex.EncodeToString(sum[:])[:hashLen]
}

// isHashed reports whether a name was capped, and so cannot be decoded back
// to its namespace.
func isHashed(name string) bool {
	return strings.Contains(name, hashedMarker)
}

// escapeRefDir is the workdir layout used before identities were one
// segment: '/' kept as nesting, only what Windows refuses escaped. Workdirs
// it named are still found.
func escapeRefDir(ref string) string {
	components := strings.Split(ref, "/")
	for i, component := range components {
		components[i] = escapeComponent(component, i > 0)
	}
	return strings.Join(components, "/")
}

// escapeComponent encodes one path component. standalone is false for the
// first ref component, which is joined onto "repo@" and so can never be a
// bare device name.
func escapeComponent(
	component string,
	standalone bool,
) string {
	var b strings.Builder
	for i := 0; i < len(component); i++ {
		if mustEscape(component, i, standalone) {
			fmt.Fprintf(&b, "%%%02X", component[i])
			continue
		}
		b.WriteByte(component[i])
	}
	return b.String()
}

// mustEscape also covers a trailing '.' or ' ', which Windows silently strips,
// and so is what keeps "." and ".." from ever reaching the filesystem.
func mustEscape(
	component string,
	i int,
	standalone bool,
) bool {
	c := component[i]
	if c < 0x20 || strings.IndexByte(windowsReserved, c) >= 0 {
		return true
	}
	if i == len(component)-1 && (c == '.' || c == ' ') {
		return true
	}
	return i == 0 && standalone && isWindowsDeviceName(component)
}

func isWindowsDeviceName(component string) bool {
	stem, _, _ := strings.Cut(component, ".")
	stem = strings.ToUpper(stem)
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(stem) != 4 || (stem[:3] != "COM" && stem[:3] != "LPT") {
		return false
	}
	return stem[3] >= '1' && stem[3] <= '9'
}

// decodeNSDir reverses either directory layout back to the namespace. A
// component that does not decode predates the encoding and is kept verbatim.
func decodeNSDir(rel string) domain.Namespace {
	bare, ref, hasRef := strings.Cut(rel, "@")
	if !hasRef {
		return domain.Namespace(rel)
	}
	components := strings.Split(ref, "/")
	for i, component := range components {
		decoded, err := url.PathUnescape(component)
		if err != nil {
			return domain.Namespace(rel)
		}
		components[i] = decoded
	}
	return domain.Namespace(bare + "@" + strings.Join(components, "/"))
}

// namespacePath resolves ns to its directory under namespacesPath, refusing
// any namespace whose bare segments would climb out of it. The identity is
// one directory, so no identity's workdir ever lies inside another's. A
// workdir an earlier layout created (the selector nested on '/', or not
// escaped at all) is used where it still exists with exactly that spelling,
// so an installed arrow never loses its files to a layout change. A path
// that exists only through case folding belongs to another identity and is
// refused.
func (s *store) namespacePath(ns domain.Namespace) (string, error) {
	current, err := s.underNamespaces(identityDir(ns))
	if err != nil {
		return "", err
	}
	if hasExactEntry(current) {
		return current, nil
	}
	for _, rel := range legacyIdentityDirs(ns) {
		legacy, err := s.underNamespaces(rel)
		if err == nil && s.existsExactly(rel) {
			return legacy, nil
		}
	}
	if exists(current) {
		return "", ErrWorkDirCollision
	}
	return current, nil
}

// legacyIdentityDirs lists the directories earlier layouts gave ns that
// differ from its current one: escaped and nested on '/', then raw.
func legacyIdentityDirs(ns domain.Namespace) []string {
	current := identityDir(ns)
	var dirs []string
	for _, rel := range []string{legacyIdentityDir(ns), string(ns)} {
		if rel != current && !slices.Contains(dirs, rel) {
			dirs = append(dirs, rel)
		}
	}
	return dirs
}

// hasExactEntry reports whether path's own directory entry exists under
// exactly its spelling.
func hasExactEntry(path string) bool {
	entries, err := os.ReadDir(filepath.Dir(path))
	return err == nil && hasEntry(entries, filepath.Base(path))
}

func identityDir(ns domain.Namespace) string {
	bare, ref, hasRef := strings.Cut(string(ns), "@")
	if !hasRef {
		return bare
	}
	parent, repo := path.Split(bare)
	return parent + capName(repo+"@"+encodeRefSegment(ref), ns)
}

func legacyIdentityDir(ns domain.Namespace) string {
	bare, ref, hasRef := strings.Cut(string(ns), "@")
	if !hasRef {
		return bare
	}
	return bare + "@" + escapeRefDir(ref)
}

func (s *store) underNamespaces(rel string) (string, error) {
	base := filepath.Clean(s.namespacesPath)
	resolved := filepath.Join(base, filepath.FromSlash(rel))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", ErrInvalidNamespace
	}
	return resolved, nil
}

// existsExactly reports whether rel exists under namespacesPath with exactly
// this spelling: on a case-insensitive filesystem a plain stat would also
// find another identity's directory that differs only by case.
func (s *store) existsExactly(rel string) bool {
	dir := filepath.Clean(s.namespacesPath)
	for _, component := range strings.Split(rel, "/") {
		entries, err := os.ReadDir(dir)
		if err != nil || !hasEntry(entries, component) {
			return false
		}
		dir = filepath.Join(dir, component)
	}
	return true
}

func hasEntry(
	entries []os.DirEntry,
	name string,
) bool {
	for _, e := range entries {
		if e.Name() == name {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}
