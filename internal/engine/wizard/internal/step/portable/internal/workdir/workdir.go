package workdir

import "path/filepath"

func Rel(
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
