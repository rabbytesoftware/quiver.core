package shelf

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func (s *shelf) resolve(
	req applyRequest,
	kind domain.ExposeKind,
	entry domain.ExposeEntry,
) ([]candidate, string, error) {
	if entry.Path != domain.ExposeAuto {
		return declared(req, entry)
	}
	if kind == domain.ExposeKindCLI {
		return s.autoCLI(req, entry)
	}
	return s.autoDesktop(req, entry)
}

func declared(
	req applyRequest,
	entry domain.ExposeEntry,
) ([]candidate, string, error) {
	if !safeName(entry.Name) {
		return nil, reasonUnsafeName, nil
	}

	target := expandPath(entry.Path, req.workdir)
	if !insideDir(req.workdir, target) {
		return nil, reasonOutsideWorkdir, nil
	}

	reason, err := contained(req.workdir, target)
	if err != nil || reason != "" {
		return nil, reason, err
	}
	return []candidate{{name: entry.Name, target: target, declared: true}}, "", nil
}

func (s *shelf) autoCLI(
	req applyRequest,
	entry domain.ExposeEntry,
) ([]candidate, string, error) {
	found, err := s.scanExecutables(req.workdir)
	if err != nil {
		return nil, "", err
	}
	if len(found) == 0 {
		return nil, reasonNoExecutable, nil
	}
	if len(found) == 1 {
		return found, "", nil
	}

	matches := matching(found, repoName(req.bare), entry.Name)
	if len(matches) > 0 {
		return uniqueNames(matches), "", nil
	}

	top := topLevel(found)
	if len(top) == 0 {
		return nil, reasonAmbiguous, nil
	}
	return uniqueNames(top), "", nil
}

func (s *shelf) autoDesktop(
	req applyRequest,
	entry domain.ExposeEntry,
) ([]candidate, string, error) {
	found, err := s.desktopCandidates(req, entry)
	if err != nil {
		return nil, "", err
	}

	picked, reason := pickDesktop(found, repoName(req.bare), entry.Name)
	if reason != "" {
		return nil, reason, nil
	}
	if s.goos == goosDarwin {
		return []candidate{picked}, "", nil
	}
	return []candidate{renamed(picked, entry.Name)}, "", nil
}

func (s *shelf) scanExecutables(
	workdir string,
) ([]candidate, error) {
	scan := &execScan{root: workdir, windows: s.goos == goosWindows}
	if err := filepath.WalkDir(workdir, scan.visit); err != nil {
		return nil, fmt.Errorf("scan %s: %w", workdir, err)
	}
	return scan.found, nil
}

func (s *shelf) desktopCandidates(
	req applyRequest,
	entry domain.ExposeEntry,
) ([]candidate, error) {
	if found := recordCandidates(req.workdir, s.goos); len(found) > 0 {
		return found, nil
	}

	found, err := s.nativeCandidates(req)
	if err != nil || len(found) > 0 {
		return found, err
	}
	if s.goos == goosDarwin {
		return s.placedBundles(req), nil
	}

	scanned, err := s.scanExecutables(req.workdir)
	if err != nil {
		return nil, err
	}
	return matching(scanned, repoName(req.bare), entry.Name), nil
}

func (s *shelf) nativeCandidates(
	req applyRequest,
) ([]candidate, error) {
	entries, err := os.ReadDir(req.workdir)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", req.workdir, err)
	}

	suffix, wantDir := s.desktopSuffix()
	found := []candidate{}
	for _, e := range entries {
		if !desktopEntryKind(e, wantDir) || !hasSuffixFold(e.Name(), suffix) {
			continue
		}
		found = append(found, candidate{
			name:   e.Name()[:len(e.Name())-len(suffix)],
			target: filepath.Join(req.workdir, e.Name()),
			depth:  1,
		})
	}
	return found, nil
}

func (s *shelf) placedBundles(
	req applyRequest,
) []candidate {
	found := []candidate{}
	for _, dir := range req.layout.apps {
		entries, _ := os.ReadDir(dir)
		found = append(found, s.ownedBundlesIn(req, dir, entries)...)
	}
	return found
}

func (s *shelf) ownedBundlesIn(
	req applyRequest,
	dir string,
	entries []fs.DirEntry,
) []candidate {
	found := []candidate{}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), bundleExtension) {
			continue
		}
		if s.bundleOwner(filepath.Join(dir, e.Name())) != req.bare {
			continue
		}
		found = append(found, candidate{
			name:   strings.TrimSuffix(e.Name(), bundleExtension),
			target: filepath.Join(req.workdir, e.Name()),
			depth:  1,
		})
	}
	return found
}

func (s *shelf) desktopSuffix() (string, bool) {
	switch s.goos {
	case goosDarwin:
		return ".app", true
	case goosWindows:
		return ".exe", false
	}
	return ".AppImage", false
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

func pickDesktop(
	found []candidate,
	repo string,
	entryName string,
) (candidate, string) {
	if len(found) == 0 {
		return candidate{}, reasonNoDesktop
	}
	if len(found) == 1 {
		return found[0], ""
	}

	matches := matching(found, repo, entryName)
	if len(matches) == 0 {
		return candidate{}, reasonAmbiguous
	}
	return matches[0], ""
}

func renamed(
	c candidate,
	name string,
) candidate {
	if safeName(name) {
		c.name = name
	}
	return c
}

func matching(
	found []candidate,
	repo string,
	entryName string,
) []candidate {
	matches := []candidate{}
	for _, c := range found {
		if strings.EqualFold(c.name, repo) || (entryName != "" && strings.EqualFold(c.name, entryName)) {
			matches = append(matches, c)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].depth < matches[j].depth })
	return matches
}

func topLevel(
	found []candidate,
) []candidate {
	top := []candidate{}
	for _, c := range found {
		if c.depth == 1 {
			top = append(top, c)
		}
	}
	return top
}

func uniqueNames(
	candidates []candidate,
) []candidate {
	seen := map[string]bool{}
	unique := []candidate{}
	for _, c := range candidates {
		if seen[c.name] {
			continue
		}
		seen[c.name] = true
		unique = append(unique, c)
	}
	return unique
}

func repoName(
	bare domain.Namespace,
) string {
	parts := strings.Split(string(bare), domain.NamespaceSeparator)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

func hasSuffixFold(
	name string,
	suffix string,
) bool {
	return len(name) > len(suffix) && strings.EqualFold(name[len(name)-len(suffix):], suffix)
}
