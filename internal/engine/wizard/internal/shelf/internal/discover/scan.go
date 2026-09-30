package discover

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

const maxScanDepth = 4

var (
	helperDirs  = map[string]bool{"node_modules": true, "resources": true, "lib": true, "lib64": true, "locales": true}
	helperFiles = map[string]bool{"chrome-sandbox": true, "chrome_crashpad_handler": true, "crashpad_handler": true}
)

type Rule interface {
	Executable(
		name string,
		mode fs.FileMode,
	) bool
	Stem(
		name string,
	) string
}

type execBits struct{}

func ExecBits() Rule {
	return execBits{}
}

func (execBits) Executable(
	_ string,
	mode fs.FileMode,
) bool {
	return mode.Perm()&0o111 != 0
}

func (execBits) Stem(
	name string,
) string {
	return name
}

type exeSuffix struct{}

func ExeSuffix() Rule {
	return exeSuffix{}
}

func (exeSuffix) Executable(
	name string,
	_ fs.FileMode,
) bool {
	return fsguard.HasSuffixFold(name, models.ExeExt)
}

func (exeSuffix) Stem(
	name string,
) string {
	if fsguard.HasSuffixFold(name, models.ExeExt) {
		return name[:len(name)-len(models.ExeExt)]
	}
	return name
}

func IsBundle(
	dir string,
) bool {
	return strings.HasSuffix(dir, models.BundleExt)
}

type Scan struct {
	Rule   Rule
	Opaque func(dir string) bool
}

func (s Scan) Executables(
	workdir string,
) ([]models.Candidate, error) {
	w := &walk{scan: s, root: workdir}
	if err := filepath.WalkDir(workdir, w.visit); err != nil {
		return nil, fmt.Errorf("scan %s: %w", workdir, err)
	}
	return w.found, nil
}

func (s Scan) opaque(
	dir string,
) bool {
	return s.Opaque != nil && s.Opaque(dir)
}

type walk struct {
	scan  Scan
	root  string
	found []models.Candidate
}

func (w *walk) visit(
	path string,
	d fs.DirEntry,
	walkErr error,
) error {
	if walkErr != nil {
		return walkErr
	}

	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}

	depth := strings.Count(rel, string(filepath.Separator)) + 1
	if d.IsDir() {
		return w.descend(d.Name(), depth)
	}
	if !d.Type().IsRegular() {
		return nil
	}

	info, err := d.Info()
	if err != nil {
		return err
	}

	name := w.scan.Rule.Stem(d.Name())
	if isHelperFile(d.Name()) || !w.scan.Rule.Executable(d.Name(), info.Mode()) || !fsguard.SafeName(name) {
		return nil
	}

	w.found = append(w.found, models.Candidate{Name: name, Target: path, Depth: depth})
	return nil
}

func (w *walk) descend(
	name string,
	depth int,
) error {
	if depth >= maxScanDepth || w.scan.opaque(name) || strings.HasPrefix(name, ".") || helperDirs[strings.ToLower(name)] {
		return filepath.SkipDir
	}
	return nil
}

func isHelperFile(
	name string,
) bool {
	lower := strings.ToLower(name)
	if helperFiles[lower] || strings.HasSuffix(lower, ".dylib") || strings.HasSuffix(lower, ".so") {
		return true
	}
	return strings.Contains(lower, ".so.")
}
