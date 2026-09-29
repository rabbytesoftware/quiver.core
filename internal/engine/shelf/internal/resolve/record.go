package resolve

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/platform"
)

const maxRecordBytes = 1 << 20

func recordCandidates(
	workdir string,
	goos string,
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
		c, ok := recordCandidate(root, workdir, record.Apps[i], goos)
		if ok {
			found = append(found, c)
		}
	}
	return found
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
	goos string,
) (models.Candidate, bool) {
	rel, ok := localPath(app.Entry)
	if !ok {
		return models.Candidate{}, false
	}

	name, ok := recordBundleName(root, rel)
	if goos != platform.GOOSDarwin {
		name, ok = recordExecName(root, rel, app.Name, goos == platform.GOOSWindows)
	}
	if !ok {
		return models.Candidate{}, false
	}

	return models.Candidate{
		Name:    name,
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

func recordBundleName(
	root *os.Root,
	rel string,
) (string, bool) {
	base := filepath.Base(rel)
	info, err := root.Lstat(rel)
	if err != nil || !info.IsDir() || !strings.HasSuffix(base, platform.BundleExt) {
		return "", false
	}

	stem := strings.TrimSuffix(base, platform.BundleExt)
	return stem, fsguard.SafeName(stem)
}

func recordExecName(
	root *os.Root,
	rel string,
	name string,
	windows bool,
) (string, bool) {
	scan := &execScan{windows: windows}
	base := filepath.Base(rel)
	info, err := root.Stat(rel)
	if err != nil || !info.Mode().IsRegular() || !scan.executable(base, info.Mode()) {
		return "", false
	}

	if fsguard.SafeName(name) {
		return name, true
	}
	stem := scan.stem(base)
	return stem, fsguard.SafeName(stem)
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
