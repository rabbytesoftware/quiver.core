package resolve

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/ownership"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/platform"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/workfs"
)

type Resolver interface {
	Resolve(
		req models.ApplyRequest,
		kind domain.ExposeKind,
		entry domain.ExposeEntry,
	) ([]models.Candidate, string, error)
}

type resolver struct {
	goos    string
	bundles ownership.Bundles
}

func New(
	host platform.Host,
	bundles ownership.Bundles,
) Resolver {
	return &resolver{goos: host.GOOS, bundles: bundles}
}

func (r *resolver) Resolve(
	req models.ApplyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	if entry.Path != domain.ExposeAuto {
		return declared(req, entry)
	}
	if kind == domain.ExposeKindCLI {
		return r.autoCLI(req, entry)
	}
	return r.autoDesktop(req, entry)
}

func declared(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	if !fsguard.SafeName(entry.Name) {
		return nil, models.ReasonUnsafeName, nil
	}

	target := fsguard.ExpandPath(entry.Path, req.Workdir)
	if !workfs.Inside(req.Workdir, target) {
		return nil, models.ReasonOutsideWorkdir, nil
	}

	reason, err := fsguard.Contained(req.Workdir, target)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	return []models.Candidate{{Name: entry.Name, Target: target, Declared: true}}, "", nil
}

func (r *resolver) autoCLI(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	found, err := r.scanExecutables(req.Workdir)
	if err != nil {
		return nil, "", err
	}
	if len(found) == 0 {
		return nil, models.ReasonNoExecutable, nil
	}
	if len(found) == 1 {
		return found, "", nil
	}

	matches := matching(found, repoName(req.Bare), entry.Name)
	if len(matches) > 0 {
		return uniqueNames(matches), "", nil
	}

	return uniqueNames(shallowest(found)), "", nil
}

func (r *resolver) autoDesktop(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	found, err := r.desktopCandidates(req, entry)
	if err != nil {
		return nil, "", err
	}

	picked, reason := pickDesktop(found, repoName(req.Bare), entry.Name)
	if reason != "" {
		return nil, reason, nil
	}
	if r.goos == platform.GOOSDarwin {
		return []models.Candidate{picked}, "", nil
	}
	return []models.Candidate{renamed(picked, entry.Name)}, "", nil
}

func (r *resolver) desktopCandidates(
	req models.ApplyRequest,
	entry domain.ExposeEntry,
) ([]models.Candidate, error) {
	if found := recordCandidates(req.Workdir, r.goos); len(found) > 0 {
		return found, nil
	}

	found, err := r.nativeCandidates(req)
	if err != nil || len(found) > 0 {
		return found, err
	}
	if r.goos == platform.GOOSDarwin {
		return r.placedBundles(req), nil
	}

	scanned, err := r.scanExecutables(req.Workdir)
	if err != nil {
		return nil, err
	}
	return matching(scanned, repoName(req.Bare), entry.Name), nil
}

func (r *resolver) nativeCandidates(
	req models.ApplyRequest,
) ([]models.Candidate, error) {
	entries, err := os.ReadDir(req.Workdir)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", req.Workdir, err)
	}

	suffix, wantDir := r.desktopSuffix()
	found := []models.Candidate{}
	for _, e := range entries {
		if !desktopEntryKind(e, wantDir) || !fsguard.HasSuffixFold(e.Name(), suffix) {
			continue
		}
		found = append(found, models.Candidate{
			Name:   e.Name()[:len(e.Name())-len(suffix)],
			Target: filepath.Join(req.Workdir, e.Name()),
			Depth:  1,
		})
	}
	return found, nil
}

func (r *resolver) placedBundles(
	req models.ApplyRequest,
) []models.Candidate {
	found := []models.Candidate{}
	for _, dir := range req.Layout.Apps {
		entries, _ := os.ReadDir(dir)
		found = append(found, r.ownedBundlesIn(req, dir, entries)...)
	}
	return found
}

func (r *resolver) ownedBundlesIn(
	req models.ApplyRequest,
	dir string,
	entries []fs.DirEntry,
) []models.Candidate {
	found := []models.Candidate{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), platform.BundleExt) {
			continue
		}
		if r.bundles.Owner(filepath.Join(dir, e.Name())).Namespace != req.Bare {
			continue
		}
		found = append(found, models.Candidate{
			Name:   strings.TrimSuffix(e.Name(), platform.BundleExt),
			Target: filepath.Join(req.Workdir, e.Name()),
			Depth:  1,
		})
	}
	return found
}

func (r *resolver) desktopSuffix() (string, bool) {
	switch r.goos {
	case platform.GOOSDarwin:
		return platform.BundleExt, true
	case platform.GOOSWindows:
		return platform.ExeExt, false
	}
	return platform.AppImageExt, false
}

func desktopEntryKind(
	e fs.DirEntry,
	wantDir bool,
) bool {
	if wantDir {
		return e.IsDir()
	}
	return e.Type().IsRegular()
}
