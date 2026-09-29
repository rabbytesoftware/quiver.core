package resolve

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
)

const maxScanDepth = 4

type execScan struct {
	root    string
	windows bool
	darwin  bool
	found   []models.Candidate
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
	if !e.executable(d.Name(), info.Mode()) || !fsguard.SafeName(name) {
		return nil
	}

	e.found = append(e.found, models.Candidate{Name: name, Target: path, Depth: depth})
	return nil
}

func (e *execScan) descend(
	name string,
	depth int,
) error {
	skipBundle := e.darwin && strings.HasSuffix(name, platform.BundleExt)
	if depth >= maxScanDepth || skipBundle || strings.HasPrefix(name, ".") {
		return filepath.SkipDir
	}
	return nil
}

func (e *execScan) executable(
	name string,
	mode fs.FileMode,
) bool {
	if e.windows {
		return fsguard.HasSuffixFold(name, platform.ExeExt)
	}
	return mode.Perm()&0o111 != 0
}

func (e *execScan) stem(
	name string,
) string {
	if e.windows && fsguard.HasSuffixFold(name, platform.ExeExt) {
		return name[:len(name)-len(platform.ExeExt)]
	}
	return name
}

func (r *resolver) scanExecutables(
	workdir string,
) ([]models.Candidate, error) {
	scan := &execScan{root: workdir, windows: r.goos == platform.GOOSWindows, darwin: r.goos == platform.GOOSDarwin}
	if err := filepath.WalkDir(workdir, scan.visit); err != nil {
		return nil, fmt.Errorf("scan %s: %w", workdir, err)
	}
	return scan.found, nil
}
