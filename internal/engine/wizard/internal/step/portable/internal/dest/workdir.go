package dest

import "path/filepath"

// WorkdirRel is path relative to workDir, slash-separated, when path sits
// strictly inside it. Only the parent is resolved: a symlink at path itself
// is judged by where it lives, not where it points.
func WorkdirRel(
	workDir string,
	path string,
) (string, bool) {
	rel, err := filepath.Rel(resolved(workDir), resolvedParent(path))
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func resolvedParent(
	path string,
) string {
	return filepath.Join(resolved(filepath.Dir(path)), filepath.Base(path))
}

func resolved(
	path string,
) string {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return target
}
