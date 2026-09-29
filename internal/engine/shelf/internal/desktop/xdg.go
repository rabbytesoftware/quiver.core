package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

const (
	xdgMarker    = "X-Quiver-Namespace="
	xdgPrefix    = "quiver-"
	xdgExtension = ".desktop"
	xdgReserved  = "\"`$\\"
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
	req models.ApplyRequest,
	entry domain.ExposeEntry,
	c models.Candidate,
) (models.Placement, error) {
	if fsguard.UnsafePath(c.Target, xdgReserved) {
		return models.Placement{Refused: models.ReasonUnsafePath}, nil
	}

	reason, err := fsguard.RequireTarget(c.Target, false)
	if err != nil || reason != "" {
		return models.Placement{Refused: reason}, err
	}

	dir := xdgDir(req.Layout.UserHome)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- the XDG applications dir is shared and must be traversable
		return models.Placement{}, fmt.Errorf("create %s: %w", dir, err)
	}

	loc := filepath.Join(dir, xdgFileName(req.Bare, c.Name))
	h, err := ownership.FileHolder(loc, xdgMarker)
	if err != nil {
		return models.Placement{}, err
	}
	if refusal := h.Refusal(req.Bare); refusal != "" {
		return models.Placement{Refused: refusal}, nil
	}

	if err := markAppImage(req.Workdir, c.Target); err != nil {
		return models.Placement{}, err
	}

	content := xdgContent(req.Bare, c.DisplayName(), c.Target, desktopIcon(req, entry, c), desktopCategories(entry.Categories))
	if err := fsguard.SwapFile(loc, []byte(content)); err != nil {
		return models.Placement{}, err
	}
	return models.Placement{Location: loc}, nil
}

func markAppImage(
	workdir string,
	target string,
) error {
	if !fsguard.HasSuffixFold(target, platform.AppImageExt) || !fsguard.ResolvesInside(workdir, target) {
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
	l platform.Layout,
	bare domain.Namespace,
	keep map[string]bool,
) error {
	return ownership.RemoveMarked(xdgDir(l.UserHome), xdgPrefix, xdgExtension, xdgMarker, bare, keep)
}
