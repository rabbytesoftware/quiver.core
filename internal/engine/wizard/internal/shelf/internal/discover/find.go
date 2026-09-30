package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

func Commands(
	req models.Request,
	entry domain.ExposeEntry,
	scan Scan,
) ([]models.Candidate, string, error) {
	found, err := scan.Executables(req.Workdir)
	if err != nil {
		return nil, "", err
	}
	if len(found) == 0 {
		return nil, models.ReasonNoExecutable, nil
	}
	if len(found) == 1 {
		return found, "", nil
	}

	matches := matching(found, RepoName(req.Bare), entry.Name)
	if len(matches) > 0 {
		return uniqueNames(matches), "", nil
	}
	return uniqueNames(shallowest(found)), "", nil
}

func Launchers(
	req models.Request,
	entry domain.ExposeEntry,
	scan Scan,
	suffix string,
) ([]models.Candidate, string, error) {
	found, err := launcherCandidates(req, entry, scan, suffix)
	if err != nil {
		return nil, "", err
	}

	picked, reason := Pick(found, RepoName(req.Bare), entry.Name)
	if reason != "" {
		return nil, reason, nil
	}
	return []models.Candidate{Renamed(picked, entry.Name)}, "", nil
}

func launcherCandidates(
	req models.Request,
	entry domain.ExposeEntry,
	scan Scan,
	suffix string,
) ([]models.Candidate, error) {
	if found := Record(req.Workdir, scan.ExecName); len(found) > 0 {
		return found, nil
	}

	found, err := Native(req.Workdir, suffix, false)
	if err != nil || len(found) > 0 {
		return found, err
	}

	scanned, err := scan.Executables(req.Workdir)
	if err != nil {
		return nil, err
	}
	matches := matching(scanned, RepoName(req.Bare), entry.Name)
	if len(matches) == 0 && len(scanned) == 1 {
		return scanned, nil
	}
	return matches, nil
}

func Native(
	workdir string,
	suffix string,
	wantDir bool,
) ([]models.Candidate, error) {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", workdir, err)
	}

	found := []models.Candidate{}
	for _, e := range entries {
		if !entryKind(e, wantDir) || !fsguard.HasSuffixFold(e.Name(), suffix) {
			continue
		}
		found = append(found, models.Candidate{
			Name:   e.Name()[:len(e.Name())-len(suffix)],
			Target: filepath.Join(workdir, e.Name()),
			Depth:  1,
		})
	}
	return found, nil
}

func Nested(
	workdir string,
	suffix string,
) ([]models.Candidate, error) {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", workdir, err)
	}

	found := []models.Candidate{}
	for _, e := range entries {
		if !e.IsDir() || fsguard.HasSuffixFold(e.Name(), suffix) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		inner, err := Native(filepath.Join(workdir, e.Name()), suffix, true)
		if err != nil {
			return nil, err
		}
		for _, c := range inner {
			c.Depth = 2
			found = append(found, c)
		}
	}
	return found, nil
}

func entryKind(
	e fs.DirEntry,
	wantDir bool,
) bool {
	if wantDir {
		return e.IsDir()
	}
	return e.Type().IsRegular()
}
