package shelf

import (
	"io/fs"
	"path/filepath"
	"strings"
)

const maxScanDepth = 3

type execScan struct {
	root    string
	windows bool
	found   []candidate
}

func (e *execScan) visit(
	path string,
	d fs.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return walkErr
	}

	rel, err := filepath.Rel(e.root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	depth := strings.Count(rel, string(filepath.Separator)) + 1
	if d.IsDir() {
		return e.descend(d.Name(), depth)
	}
	if !d.Type().IsRegular() {
		return nil
	}

	info, err := d.Info()
	if err != nil {
		return err
	}

	name := e.stem(d.Name())
	if !e.executable(d.Name(), info.Mode()) || !safeName(name) {
		return nil
	}

	e.found = append(e.found, candidate{name: name, target: path, depth: depth})
	return nil
}

func (e *execScan) descend(
	name string,
	depth int,
) error {
	if depth >= maxScanDepth || strings.HasSuffix(name, ".app") || strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}
	return nil
}

func (e *execScan) executable(
	name string,
	mode fs.FileMode,
) bool {
	if e.windows {
		return hasSuffixFold(name, ".exe")
	}
	return mode.Perm()&0o111 != 0
}

func (e *execScan) stem(
	name string,
) string {
	if e.windows && hasSuffixFold(name, ".exe") {
		return name[:len(name)-len(".exe")]
	}
	return name
}
