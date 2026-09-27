package forge

import (
	"strings"
)

const (
	exposeNameLimit    = 64
	exposeNameFallback = "app"
)

func exposeName(
	repo string,
) string {
	var b strings.Builder
	for _, r := range repo {
		b.WriteRune(exposeRune(r))
	}
	name := strings.TrimLeft(b.String(), "._+-")
	if len(name) > exposeNameLimit {
		name = name[:exposeNameLimit]
	}
	if name == "" {
		return exposeNameFallback
	}
	return name
}

func exposeRune(
	r rune,
) rune {
	if isAlnum(r) || strings.ContainsRune("._+-", r) {
		return r
	}
	return '-'
}

func isAlnum(
	r rune,
) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}
