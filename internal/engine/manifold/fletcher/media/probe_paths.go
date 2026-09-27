package media

import "strings"

func IconProbePaths() []string {
	return []string{
		"src-tauri/icons/icon.png",
		"build/icon.png",
		"resources/icon.png",
		"assets/icon.png",
		"assets/logo.svg",
		"logo.svg",
		".github/logo.svg",
		".github/logo.png",
		"docs/logo.svg",
		"icon.png",
		"logo.png",
	}
}

func isSVGPath(
	path string,
) bool {
	return strings.HasSuffix(strings.ToLower(path), ".svg")
}

func isRejectedIconPath(
	path string,
) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".ico") || strings.HasSuffix(lower, ".icns") {
		return true
	}
	return strings.Contains(lower, "favicon")
}

func acceptProbedIcon(
	path string,
	data []byte,
) bool {
	if isRejectedIconPath(path) {
		return false
	}
	if isSVGPath(path) {
		return true
	}
	dim, ok := Sniff(data)
	if !ok {
		return false
	}
	return dim.Width == dim.Height && dim.Width >= 128
}
