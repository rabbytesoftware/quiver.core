package vault

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// windowsReserved holds the characters Windows refuses in a path component,
// plus '%' so an escaped component always decodes back to exactly one ref.
const windowsReserved = `<>:"|?*\%`

// encodeNS flattens a namespace into a single filename. url.PathEscape leaves
// ':' alone since it is legal in a URL path segment, but Windows rejects it,
// and only a selector identity (never a git ref) can carry one.
func encodeNS(ns domain.Namespace) string {
	bare, ref, hasRef := strings.Cut(string(ns), "@")
	if !hasRef {
		return url.PathEscape(bare)
	}
	return url.PathEscape(bare) + "@" + strings.ReplaceAll(url.PathEscape(ref), ":", "%3A")
}

// escapeRefDir percent-encodes the parts of a ref a directory tree cannot
// hold, keeping '/' as nesting. git check-ref-format already forbids
// everything rewritten here, so an existing ref's layout is untouched; only
// selector identities such as v1.* are affected.
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

// decodeNSDir reverses the directory layout back to the namespace. A
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
// any namespace whose bare segments would climb out of it.
func (s *store) namespacePath(ns domain.Namespace) (string, error) {
	rel := string(ns)
	if bare, ref, hasRef := strings.Cut(rel, "@"); hasRef {
		rel = bare + "@" + escapeRefDir(ref)
	}
	base := filepath.Clean(s.namespacesPath)
	resolved := filepath.Join(base, filepath.FromSlash(rel))
	if !strings.HasPrefix(resolved, base+string(filepath.Separator)) {
		return "", ErrInvalidNamespace
	}
	return resolved, nil
}
