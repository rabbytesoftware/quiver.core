package bundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/discover"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

func (e *exposer) Find(
	req models.Request,
	entry domain.ExposeEntry,
) ([]models.Candidate, string, error) {
	found, err := e.candidates(req)
	if err != nil {
		return nil, "", err
	}

	picked, reason := discover.Pick(found, discover.RepoName(req.Bare), entry.Name)
	if reason != "" {
		return nil, reason, nil
	}
	return []models.Candidate{picked}, "", nil
}

func (e *exposer) candidates(
	req models.Request,
) ([]models.Candidate, error) {
	if found := discover.Record(req.Workdir, discover.BundleName); len(found) > 0 {
		return found, nil
	}

	found, err := discover.Native(req.Workdir, models.BundleExt, true)
	if err != nil || len(found) > 0 {
		return found, err
	}
	return e.placed(req), nil
}

func (e *exposer) placed(
	req models.Request,
) []models.Candidate {
	found := []models.Candidate{}
	for _, dir := range req.Layout.Apps {
		entries, _ := os.ReadDir(dir)
		found = append(found, e.ownedIn(req, dir, entries)...)
	}
	return found
}

func (e *exposer) ownedIn(
	req models.Request,
	dir string,
	entries []fs.DirEntry,
) []models.Candidate {
	found := []models.Candidate{}
	for _, d := range entries {
		if !d.IsDir() || !strings.HasSuffix(d.Name(), models.BundleExt) {
			continue
		}
		if e.bundles.Owner(filepath.Join(dir, d.Name())).Namespace != req.Bare {
			continue
		}
		found = append(found, models.Candidate{
			Name:   strings.TrimSuffix(d.Name(), models.BundleExt),
			Target: filepath.Join(req.Workdir, d.Name()),
			Depth:  1,
		})
	}
	return found
}
