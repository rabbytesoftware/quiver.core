package forge

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

var safeFileNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

func LocalFile(
	pick picker.Pick,
) (string, bool) {
	if pick.Asset.Digest == "" {
		return "", false
	}
	u, err := url.Parse(pick.Asset.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	name, err := url.PathUnescape(path.Base(u.EscapedPath()))
	if err != nil || !safeFileName(name) {
		return "", false
	}
	return name, true
}

func safeFileName(
	name string,
) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	return safeFileNamePattern.MatchString(name)
}
