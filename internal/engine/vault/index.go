package vault

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const defaultSearchLimit = 25

type index struct {
	db *gorm.DB
}

// openIndex opens the vault's read model, creating its schema if absent.
// The FTS5 table is created with raw DDL because GORM cannot AutoMigrate
// virtual tables.
func openIndex(dbPath string) (*index, error) {
	db, err := adapterSQLite.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("vault index: open: %w", err)
	}

	if err := db.AutoMigrate(
		&arrowIndexRow{},
		&arrowTagRow{},
		&arrowOSRow{},
	); err != nil {
		return nil, fmt.Errorf("vault index: migrate: %w", err)
	}

	if err := db.Exec(
		`CREATE VIRTUAL TABLE IF NOT EXISTS vault_arrows_fts USING fts5(
			namespace UNINDEXED,
			ref UNINDEXED,
			name,
			description,
			tags,
			tokenize = 'trigram'
		)`,
	).Error; err != nil {
		return nil, fmt.Errorf("vault index: create fts: %w", err)
	}

	return &index{db: db}, nil
}

func (i *index) close() error {
	sqlDB, err := i.db.DB()
	if err != nil {
		return fmt.Errorf("vault index: close: %w", err)
	}
	return sqlDB.Close()
}

// upsert writes one namespace@ref, replacing whatever was there. Both now and
// ttl are supplied by the caller so the index holds no clock of its own; ttl
// slides row_expire_at forward on every re-save.
func (i *index) upsert(
	ns domain.Namespace,
	file ManifestFile,
	meta IndexMeta,
	now time.Time,
	ttl time.Duration,
) error {
	bare := ns.BareNamespace().String()
	ref := ns.Ref()

	return i.db.Transaction(func(tx *gorm.DB) error {
		row := arrowIndexRow{
			Namespace:   bare,
			Ref:         ref,
			Name:        meta.Arrow.Name,
			Description: meta.Arrow.Description,
			Icon:        meta.Arrow.Media.Icon,
			Banner:      meta.Arrow.Media.Banner,
			Stars:       meta.Stars,
			Source:      meta.Source,
			Filename:    file.Filename,
			Branch:      meta.Branch,
			SeenAt:      now.Unix(),
			RowExpireAt: now.Add(ttl).Unix(),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "namespace"}, {Name: "ref"}},
			UpdateAll: true,
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("vault index: upsert row: %w", err)
		}

		if err := replaceChildRows(tx, bare, ref, meta); err != nil {
			return err
		}
		return replaceFTS(tx, bare, ref, meta)
	})
}

func replaceChildRows(
	tx *gorm.DB,
	bare string,
	ref string,
	meta IndexMeta,
) error {
	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowTagRow{}).Error; err != nil {
		return fmt.Errorf("vault index: clear tags: %w", err)
	}
	for _, tag := range meta.Arrow.Tags {
		if err := tx.Create(&arrowTagRow{Namespace: bare, Ref: ref, Tag: tag}).Error; err != nil {
			return fmt.Errorf("vault index: write tag: %w", err)
		}
	}

	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowOSRow{}).Error; err != nil {
		return fmt.Errorf("vault index: clear os: %w", err)
	}
	for _, os := range meta.OS {
		if err := tx.Create(&arrowOSRow{Namespace: bare, Ref: ref, OS: string(os)}).Error; err != nil {
			return fmt.Errorf("vault index: write os: %w", err)
		}
	}
	return nil
}

func replaceFTS(
	tx *gorm.DB,
	bare string,
	ref string,
	meta IndexMeta,
) error {
	if err := tx.Exec(
		`DELETE FROM vault_arrows_fts WHERE namespace = ? AND ref = ?`,
		bare, ref,
	).Error; err != nil {
		return fmt.Errorf("vault index: clear fts: %w", err)
	}
	if err := tx.Exec(
		`INSERT INTO vault_arrows_fts (namespace, ref, name, description, tags)
		 VALUES (?, ?, ?, ?, ?)`,
		bare, ref, meta.Arrow.Name, meta.Arrow.Description,
		strings.Join(meta.Arrow.Tags, " "),
	).Error; err != nil {
		return fmt.Errorf("vault index: write fts: %w", err)
	}
	return nil
}

type searchScan struct {
	Namespace   string
	Ref         string
	Name        string
	Description string
	Icon        string
	Banner      string
	Stars       int
	Source      string
	Branch      string
	SeenAt      int64
}

// search matches rows by trigram FTS, ranked by bm25 with name weighted above
// tags above description. Rows past row_expire_at are excluded even if the
// sweep has not yet removed them.
func (i *index) search(
	q IndexQuery,
	now time.Time,
) ([]IndexRow, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return []IndexRow{}, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	sql := `
		SELECT a.namespace, a.ref, a.name, a.description, a.icon, a.banner,
		       a.stars, a.source, a.branch, a.seen_at
		FROM vault_arrows_fts f
		JOIN vault_arrows a ON a.namespace = f.namespace AND a.ref = f.ref
		WHERE vault_arrows_fts MATCH ?
		  AND a.row_expire_at > ?`
	args := []any{ftsPhrase(text), now.Unix()}

	if q.OS != "" {
		sql += ` AND EXISTS (
			SELECT 1 FROM vault_arrow_os o
			WHERE o.namespace = a.namespace AND o.ref = a.ref AND o.os = ?
		)`
		args = append(args, string(q.OS))
	}

	// bm25 weights are positional per FTS5 column order; unindexed columns get 0.
	sql += ` ORDER BY bm25(vault_arrows_fts, 0.0, 0.0, 10.0, 2.0, 5.0) LIMIT ?`
	args = append(args, limit)

	var scanned []searchScan
	if err := i.db.Raw(sql, args...).Scan(&scanned).Error; err != nil {
		return nil, fmt.Errorf("vault index: search: %w", err)
	}

	return i.hydrate(scanned)
}

// ftsPhrase wraps text as a single FTS5 quoted phrase, so punctuation that is
// query syntax to FTS5 (-, ", *, OR) is matched literally instead of parsed.
func ftsPhrase(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

func (i *index) hydrate(scanned []searchScan) ([]IndexRow, error) {
	if len(scanned) == 0 {
		return []IndexRow{}, nil
	}

	where, args := keyPredicate(scanned)

	var tagRows []arrowTagRow
	if err := i.db.Where(where, args...).Order("tag").Find(&tagRows).Error; err != nil {
		return nil, fmt.Errorf("vault index: load tags: %w", err)
	}
	tags := make(map[string][]string, len(scanned))
	for _, r := range tagRows {
		k := rowKey(r.Namespace, r.Ref)
		tags[k] = append(tags[k], r.Tag)
	}

	var osRows []arrowOSRow
	if err := i.db.Where(where, args...).Order("os").Find(&osRows).Error; err != nil {
		return nil, fmt.Errorf("vault index: load os: %w", err)
	}
	oses := make(map[string][]domain.OS, len(scanned))
	for _, r := range osRows {
		k := rowKey(r.Namespace, r.Ref)
		oses[k] = append(oses[k], domain.OS(r.OS))
	}

	rows := make([]IndexRow, 0, len(scanned))
	for _, s := range scanned {
		k := rowKey(s.Namespace, s.Ref)
		rows = append(rows, IndexRow{
			Namespace: domain.Namespace(s.Namespace),
			Ref:       s.Ref,
			SeenAt:    time.Unix(s.SeenAt, 0).UTC(),
			Meta: IndexMeta{
				Arrow: domain.ArrowMeta{
					Name:        s.Name,
					Description: s.Description,
					Tags:        tags[k],
					Media:       domain.ArrowMedia{Icon: s.Icon, Banner: s.Banner},
				},
				OS:     oses[k],
				Stars:  s.Stars,
				Source: s.Source,
				Branch: s.Branch,
			},
		})
	}
	return rows, nil
}

func rowKey(
	bare string,
	ref string,
) string {
	return bare + "@" + ref
}

// keyPredicate builds one row-value IN clause covering every scanned key, so
// child rows load in a single query rather than one per result.
func keyPredicate(scanned []searchScan) (string, []any) {
	tuples := make([]string, 0, len(scanned))
	args := make([]any, 0, len(scanned)*2)
	for _, s := range scanned {
		tuples = append(tuples, "(?, ?)")
		args = append(args, s.Namespace, s.Ref)
	}
	return "(namespace, ref) IN (VALUES " + strings.Join(tuples, ", ") + ")", args
}

func (i *index) evictExpired(now time.Time) error {
	return i.db.Transaction(func(tx *gorm.DB) error {
		var expired []arrowIndexRow
		if err := tx.Where("row_expire_at <= ?", now.Unix()).Find(&expired).Error; err != nil {
			return fmt.Errorf("vault index: select expired: %w", err)
		}
		for _, row := range expired {
			if err := deleteKey(tx, row.Namespace, row.Ref); err != nil {
				return err
			}
		}
		return nil
	})
}

// forget removes every ref under a bare namespace.
func (i *index) forget(ns domain.Namespace) error {
	bare := ns.BareNamespace().String()

	return i.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("namespace = ?", bare).Delete(&arrowIndexRow{}).Error; err != nil {
			return fmt.Errorf("vault index: forget row: %w", err)
		}
		if err := tx.Where("namespace = ?", bare).Delete(&arrowTagRow{}).Error; err != nil {
			return fmt.Errorf("vault index: forget tags: %w", err)
		}
		if err := tx.Where("namespace = ?", bare).Delete(&arrowOSRow{}).Error; err != nil {
			return fmt.Errorf("vault index: forget os: %w", err)
		}
		if err := tx.Exec(
			`DELETE FROM vault_arrows_fts WHERE namespace = ?`, bare,
		).Error; err != nil {
			return fmt.Errorf("vault index: forget fts: %w", err)
		}
		return nil
	})
}

func deleteKey(
	tx *gorm.DB,
	bare string,
	ref string,
) error {
	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowIndexRow{}).Error; err != nil {
		return fmt.Errorf("vault index: delete row: %w", err)
	}
	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowTagRow{}).Error; err != nil {
		return fmt.Errorf("vault index: delete tags: %w", err)
	}
	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowOSRow{}).Error; err != nil {
		return fmt.Errorf("vault index: delete os: %w", err)
	}
	if err := tx.Exec(
		`DELETE FROM vault_arrows_fts WHERE namespace = ? AND ref = ?`, bare, ref,
	).Error; err != nil {
		return fmt.Errorf("vault index: delete fts: %w", err)
	}
	return nil
}
