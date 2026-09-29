package appimage

import (
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	desktopEntryGroup   = "[Desktop Entry]"
	maxDesktopFileBytes = 1 << 20
	fieldCodes          = "fFuUdDnNickvm"
	hicolorDir          = "usr/share/icons/hicolor"
	dirIcon             = ".DirIcon"
)

type desktopEntry struct {
	name string
	exec string
	icon string
}

func readCappedDesktopFile(
	r io.Reader,
) (desktopEntry, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDesktopFileBytes+1))
	if err != nil {
		return desktopEntry{}, fmt.Errorf("unpack: appimage metadata: %w", err)
	}
	if len(data) > maxDesktopFileBytes {
		return desktopEntry{}, nil
	}

	return parseDesktopFile(data), nil
}

func parseDesktopFile(
	data []byte,
) desktopEntry {
	var entry desktopEntry
	inGroup := false
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			inGroup = line == desktopEntryGroup
		case inGroup:
			entry.set(line)
		}
	}

	return entry
}

func (e *desktopEntry) set(
	line string,
) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return
	}

	value = strings.TrimSpace(value)
	switch strings.TrimSpace(key) {
	case "Name":
		e.name = cmp.Or(e.name, value)
	case "Exec":
		e.exec = cmp.Or(e.exec, value)
	case "Icon":
		e.icon = cmp.Or(e.icon, value)
	}
}

func parseExec(
	line string,
) (string, []string) {
	tokens := splitExec(line)
	if len(tokens) == 0 {
		return "", nil
	}

	args := make([]string, 0, len(tokens)-1)
	for _, token := range tokens[1:] {
		if isFieldCode(token) {
			continue
		}
		args = append(args, strings.ReplaceAll(token, "%%", "%"))
	}

	return tokens[0], args
}

func splitExec(
	line string,
) []string {
	var tokens []string
	var current strings.Builder
	started := false
	quoted := false

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quoted && r == '\\' && i+1 < len(runes) && isQuotedEscape(runes[i+1]):
			i++
			current.WriteRune(runes[i])
		case r == '"':
			quoted = !quoted
			started = true
		case !quoted && (r == ' ' || r == '\t'):
			if started {
				tokens = append(tokens, current.String())
			}
			current.Reset()
			started = false
		default:
			current.WriteRune(r)
			started = true
		}
	}

	if started {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func isQuotedEscape(
	r rune,
) bool {
	return r == '"' || r == '`' || r == '$' || r == '\\'
}

func isFieldCode(
	token string,
) bool {
	return len(token) == 2 && token[0] == '%' && strings.IndexByte(fieldCodes, token[1]) >= 0
}

func resolveIcon(
	root *os.Root,
	name string,
) string {
	if strings.ContainsAny(name, `/\`) {
		return ""
	}

	if icon := namedIcon(root, name); icon != "" {
		return icon
	}

	info, err := root.Lstat(dirIcon)
	if err == nil && info.Mode().IsRegular() {
		return dirIcon
	}

	return ""
}

func namedIcon(
	root *os.Root,
	name string,
) string {
	if name == "" {
		return ""
	}

	for _, candidate := range iconCandidates(root, name) {
		if isRegularInside(root, candidate) {
			return candidate
		}
	}

	return ""
}

func iconCandidates(
	root *os.Root,
	name string,
) []string {
	candidates := []string{name + ".png", name + ".svg", name + ".xpm"}
	for _, size := range hicolorSizes(root) {
		candidates = append(candidates, path.Join(hicolorDir, fmt.Sprintf("%dx%d", size, size), "apps", name+".png"))
	}

	return append(candidates, path.Join(hicolorDir, "scalable", "apps", name+".svg"))
}

func hicolorSizes(
	root *os.Root,
) []int {
	entries, err := fs.ReadDir(root.FS(), hicolorDir)
	if err != nil {
		return nil
	}

	sizes := make([]int, 0, len(entries))
	for _, e := range entries {
		if size, ok := squareSize(e.Name()); ok {
			sizes = append(sizes, size)
		}
	}
	slices.Sort(sizes)
	slices.Reverse(sizes)

	return sizes
}

func squareSize(
	name string,
) (int, bool) {
	width, height, ok := strings.Cut(name, "x")
	if !ok || width != height {
		return 0, false
	}

	size, err := strconv.Atoi(width)

	return size, err == nil && size > 0
}
