package guard

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

const windowsInvalidChars = `<>:"|?*`

func IsAbsolute(
	path string,
) bool {
	return strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, `\`) ||
		filepath.IsAbs(path) ||
		filepath.VolumeName(path) != ""
}

func (g *Guard) admitName(
	cleaned string,
) error {
	parts := strings.FieldsFunc(cleaned, isSeparator)
	if g.rules.WindowsNames && !windowsSafe(parts) {
		return fmt.Errorf("unpack: %q: %w", cleaned, models.ErrReservedName)
	}
	if !g.rules.FoldCase {
		return nil
	}

	return g.claimFolded(cleaned, parts)
}

func (g *Guard) claimFolded(
	cleaned string,
	parts []string,
) error {
	prefix := ""
	for _, part := range parts {
		prefix = path.Join(prefix, part)
		key := strings.ToLower(prefix)
		if seen, ok := g.seen[key]; ok && seen != prefix {
			return fmt.Errorf("unpack: %q and %q: %w", seen, cleaned, models.ErrNameCollision)
		}
		g.seen[key] = prefix
	}

	return nil
}

func windowsSafe(
	parts []string,
) bool {
	for _, part := range parts {
		if reservedOnWindows(part) {
			return false
		}
	}

	return true
}

func reservedOnWindows(
	part string,
) bool {
	return domain.IsWindowsReservedName(part) ||
		strings.HasSuffix(part, ".") ||
		strings.HasSuffix(part, " ") ||
		strings.ContainsAny(part, windowsInvalidChars) ||
		strings.ContainsFunc(part, unicode.IsControl)
}

func isSeparator(
	r rune,
) bool {
	return r == '/' || r == '\\'
}

func entryName(
	name string,
) (string, error) {
	cleaned, err := cleanName(name)
	if err != nil {
		return "", err
	}

	if cleaned == "." {
		return "", fmt.Errorf("unpack: %q: invalid entry name", name)
	}

	return cleaned, nil
}

func cleanName(
	name string,
) (string, error) {
	if IsAbsolute(name) {
		return "", fmt.Errorf("unpack: %q: absolute path: %w", name, models.ErrEscape)
	}

	cleaned := filepath.Clean(filepath.FromSlash(name))
	if !workfs.RelInside(cleaned) {
		return "", fmt.Errorf("unpack: %q: %w", name, models.ErrEscape)
	}

	return cleaned, nil
}
