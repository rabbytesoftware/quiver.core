package unpack

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	hicolorDir = "usr/share/icons/hicolor"
	dirIcon    = ".DirIcon"
)

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
