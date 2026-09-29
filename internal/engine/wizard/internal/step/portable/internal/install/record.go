package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func (i *installer) Record(
	ctx context.Context,
	nsKey string,
	workDir string,
	apps []domain.PortableApp,
) error {
	if len(apps) == 0 {
		return nil
	}

	relative, outside := relativeApps(workDir, apps)
	if outside != "" {
		slog.WarnContext(ctx, "portable: app outside the workdir, not recorded", "ns", nsKey, "workdir", workDir, "path", outside)
		return nil
	}

	path := filepath.Join(workDir, domain.PortableRecordFile)
	current, err := readRecord(path)
	if err != nil {
		return err
	}

	return writeRecord(path, current.Merge(relative))
}

func relativeApps(
	workDir string,
	apps []domain.PortableApp,
) ([]domain.PortableApp, string) {
	relative := make([]domain.PortableApp, 0, len(apps))
	for _, app := range apps {
		entry, ok := workdirRel(workDir, app.Entry)
		if !ok {
			return nil, app.Entry
		}

		icon, ok := workdirRel(workDir, app.Icon)
		if app.Icon != "" && !ok {
			return nil, app.Icon
		}

		relative = append(relative, domain.PortableApp{Name: app.Name, Entry: entry, Icon: icon})
	}

	return relative, ""
}

func readRecord(
	path string,
) (domain.PortableRecord, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- record file Quiver writes under the arrow's workdir
	if errors.Is(err, os.ErrNotExist) {
		return domain.PortableRecord{}, nil
	}
	if err != nil {
		return domain.PortableRecord{}, fmt.Errorf("portable: read record: %w", err)
	}

	var record domain.PortableRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return domain.PortableRecord{}, nil
	}

	return record, nil
}

func writeRecord(
	path string,
	record domain.PortableRecord,
) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), domain.PortableRecordFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("portable: write record: %w", err)
	}

	if err := commitRecord(tmp, record, path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("portable: write record: %w", err)
	}

	return nil
}

func commitRecord(
	tmp *os.File,
	record domain.PortableRecord,
	path string,
) error {
	if err := errors.Join(json.NewEncoder(tmp).Encode(record), tmp.Close()); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
