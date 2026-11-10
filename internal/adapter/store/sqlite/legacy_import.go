package sqlite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImportLegacyDB copies every Row of the legacy database into host and
// returns how many were read. A legacy database without the table, or a nil
// one, copies nothing. A row whose key already exists in host is kept as it
// is, so repeating the import never overwrites newer state.
func ImportLegacyDB[Row any](
	host *gorm.DB,
	legacy *gorm.DB,
) (int, error) {
	var model Row
	if legacy == nil || !legacy.Migrator().HasTable(&model) {
		return 0, nil
	}
	var rows []Row
	err := legacy.Find(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("read legacy rows: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	err = host.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("copy %d rows: %w", len(rows), err)
	}
	return len(rows), nil
}

// ImportLegacyFile copies every Row of the database file at legacyPath into
// host, then keeps the file beside itself as legacyPath+".migrated" so the
// import runs once. An empty or missing path imports nothing, and a file that
// cannot be renamed is left where it is (the import is idempotent, see
// ImportLegacyDB). It returns how many rows the file held.
func ImportLegacyFile[Row any](
	host *gorm.DB,
	legacyPath string,
) (int, error) {
	if legacyPath == "" {
		return 0, nil
	}
	_, err := os.Stat(legacyPath)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", legacyPath, err)
	}

	legacy, err := OpenDB(legacyPath)
	if err != nil {
		return 0, err
	}
	copied, err := ImportLegacyDB[Row](host, legacy)
	closeErr := CloseDB(legacy)
	if err != nil {
		return 0, fmt.Errorf("import %s: %w", legacyPath, err)
	}
	if closeErr != nil {
		return 0, closeErr
	}
	SetAsideLegacyFile(legacyPath)
	return copied, nil
}

// SetAsideLegacyFile keeps the database file of an earlier release beside
// itself as legacyPath+".migrated", once its rows live in the events database.
// A file that cannot be renamed is left where it is and the failure is logged:
// the import is idempotent, so the next start only repeats it.
func SetAsideLegacyFile(
	legacyPath string,
) {
	err := os.Rename(legacyPath, legacyPath+".migrated")
	if err != nil {
		slog.WarnContext(context.Background(), "legacy read model: keep the file where it is",
			"path", legacyPath, "err", err)
		return
	}
	slog.InfoContext(context.Background(), "legacy read model: moved into the events database",
		"legacy", legacyPath+".migrated")
}
