package discover

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
)

const maxRecordBytes = 1 << 20

type Namer func(root *os.Root, rel, appName string) (string, bool)

func Record(
	workdir string,
	name Namer,
) []models.Candidate {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return nil
	}
	defer func() { _ = root.Close() }()

	record, ok := readPortableRecord(root)
	if !ok {
		return nil
	}

	found := []models.Candidate{}
	for i := len(record.Apps) - 1; i >= 0; i-- {
		c, ok := recordCandidate(root, workdir, record.Apps[i], name)
		if ok {
			found = append(found, c)
		}
	}
	return found
}

func BundleName(
	root *os.Root,
	rel string,
	_ string,
) (string, bool) {
	base := filepath.Base(rel)
	info, err := root.Lstat(rel)
	if err != nil || !info.IsDir() || !strings.HasSuffix(base, models.BundleExt) {
		return "", false
	}

	stem := strings.TrimSuffix(base, models.BundleExt)
	return stem, fsguard.SafeName(stem)
}

func (s Scan) ExecName(
	root *os.Root,
	rel string,
	appName string,
) (string, bool) {
	base := filepath.Base(rel)
	info, err := root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() || !s.Rule.Executable(base, info.Mode()) {
		return "", false
	}

	if fsguard.SafeName(appName) {
		return appName, true
	}
	stem := s.Rule.Stem(base)
	return stem, fsguard.SafeName(stem)
}

func readPortableRecord(
	root *os.Root,
) (domain.PortableRecord, bool) {
	info, err := root.Lstat(domain.PortableRecordFile)
	if err != nil || !info.Mode().IsRegular() {
		return domain.PortableRecord{}, false
	}

	f, err := root.Open(domain.PortableRecordFile)
	if err != nil {
		return domain.PortableRecord{}, false
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil || len(data) > maxRecordBytes {
		return domain.PortableRecord{}, false
	}

	var record domain.PortableRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return domain.PortableRecord{}, false
	}
	return record, true
}

func recordCandidate(
	root *os.Root,
	workdir string,
	app domain.PortableApp,
	name Namer,
) (models.Candidate, bool) {
	rel, ok := localPath(app.Entry)
	if !ok {
		return models.Candidate{}, false
	}

	named, ok := name(root, rel, app.Name)
	if !ok {
		return models.Candidate{}, false
	}

	return models.Candidate{
		Name:    named,
		Display: recordDisplay(app.Name),
		Target:  filepath.Join(workdir, rel),
		Icon:    recordIcon(root, workdir, app.Icon),
		Depth:   1,
	}, true
}

func localPath(
	raw string,
) (string, bool) {
	rel := filepath.FromSlash(raw)
	if !filepath.IsLocal(rel) {
		return "", false
	}
	return filepath.Clean(rel), true
}

func recordIcon(
	root *os.Root,
	workdir string,
	raw string,
) string {
	rel, ok := localPath(raw)
	if !ok {
		return ""
	}

	info, err := root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return filepath.Join(workdir, rel)
}

func recordDisplay(
	name string,
) string {
	if fsguard.UnsafePath(name, "") {
		return ""
	}
	return name
}
