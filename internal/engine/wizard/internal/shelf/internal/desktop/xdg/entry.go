package xdg

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
)

const (
	ownerKey   = "X-Quiver-Namespace="
	workdirKey = "X-Quiver-Workdir="
	filePrefix = "quiver-"
	fileExt    = ".desktop"
	reserved   = "\"`$\\"
)

func marker() ownership.Marker {
	return ownership.Marker{Owner: ownerKey, Workdir: workdirKey}
}

func entriesDir(
	userHome string,
) string {
	return filepath.Join(userHome, ".local", "share", "applications")
}

func fileName(
	bare domain.Namespace,
	name string,
) string {
	sum := sha256.Sum256([]byte(bare))
	return filePrefix + hex.EncodeToString(sum[:])[:12] + "-" + strings.Map(sanitizeRune, name) + fileExt
}

func sanitizeRune(
	r rune,
) rune {
	if validCategoryRune(r) || r == '.' {
		return r
	}
	return '-'
}

func content(
	bare domain.Namespace,
	workdir string,
	name string,
	target string,
	icon string,
	categories []string,
) string {
	var b strings.Builder
	b.WriteString("[Desktop Entry]\nType=Application\n")
	b.WriteString("Name=" + name + "\n")
	b.WriteString("Exec=\"" + strings.ReplaceAll(target, "%", "%%") + "\" %U\n")
	if icon != "" {
		b.WriteString("Icon=" + icon + "\n")
	}
	b.WriteString("Terminal=false\n")
	if len(categories) > 0 {
		b.WriteString("Categories=" + strings.Join(categories, ";") + ";\n")
	}
	b.WriteString(marker().Lines(bare, workdir, "\n"))
	return b.String()
}

func validCategories(
	categories []string,
) []string {
	valid := []string{}
	for _, c := range categories {
		if c != "" && strings.IndexFunc(c, invalidCategoryRune) < 0 {
			valid = append(valid, c)
		}
	}
	return valid
}

func validCategoryRune(
	r rune,
) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
}

func invalidCategoryRune(
	r rune,
) bool {
	return !validCategoryRune(r)
}
