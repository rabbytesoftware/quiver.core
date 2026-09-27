package shelf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	xdgMarker    = "X-Quiver-Namespace="
	xdgPrefix    = "quiver-"
	xdgExtension = ".desktop"
	xdgReserved  = "\"`$\\"
	appImageExt  = ".AppImage"
)

func xdgDir(
	userHome string,
) string {
	return filepath.Join(userHome, ".local", "share", "applications")
}

func xdgFileName(
	bare domain.Namespace,
	name string,
) string {
	sum := sha256.Sum256([]byte(bare))
	return xdgPrefix + hex.EncodeToString(sum[:])[:12] + "-" + strings.Map(sanitizeRune, name) + xdgExtension
}

func sanitizeRune(
	r rune,
) rune {
	if validCategoryRune(r) || r == '.' {
		return r
	}
	return '-'
}

func xdgContent(
	bare domain.Namespace,
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
	b.WriteString(xdgMarker + string(bare) + "\n")
	return b.String()
}

func desktopIcon(
	req applyRequest,
	entry domain.ExposeEntry,
	c candidate,
) string {
	icon := req.media.Icon
	if c.icon != "" {
		icon = c.icon
	}
	if entry.Icon != "" {
		icon = expandPath(entry.Icon, req.workdir)
	}
	if strings.Contains(icon, "://") || unsafePath(icon, "") {
		return ""
	}
	return icon
}

func desktopCategories(
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

func placeXDG(
	req applyRequest,
	entry domain.ExposeEntry,
	c candidate,
) (placement, error) {
	if unsafePath(c.target, xdgReserved) {
		return placement{refused: reasonUnsafePath}, nil
	}

	reason, err := requireTarget(c.target, false)
	if err != nil || reason != "" {
		return placement{refused: reason}, err
	}

	dir := xdgDir(req.layout.userHome)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, xdgFileName(req.bare, c.name))
	h, err := fileHolder(loc, xdgMarker)
	if err != nil {
		return placement{}, err
	}
	if refusal := h.refusal(req.bare); refusal != "" {
		return placement{refused: refusal}, nil
	}

	if err := markAppImage(req.workdir, c.target); err != nil {
		return placement{}, err
	}

	content := xdgContent(req.bare, c.displayName(), c.target, desktopIcon(req, entry, c), desktopCategories(entry.Categories))
	if err := swapFile(loc, []byte(content)); err != nil {
		return placement{}, err
	}
	return placement{location: loc}, nil
}

func markAppImage(
	workdir string,
	target string,
) error {
	if !hasSuffixFold(target, appImageExt) || !resolvesInside(workdir, target) {
		return nil
	}

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", target, err)
	}
	if err := os.Chmod(target, info.Mode().Perm()|0o111); err != nil {
		return fmt.Errorf("mark %s executable: %w", target, err)
	}
	return nil
}

func removeXDG(
	l layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	return removeMarked(xdgDir(l.userHome), xdgPrefix, xdgExtension, xdgMarker, bare, keep)
}
