package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// versionOrder ranks the versions of one namespace: a user-installed version
// wins, then the most recently installed, then the highest ref so ties are
// stable. The preferred version supplies the parent row's derived columns.
const versionOrder = "user_installed DESC, installed_at DESC, ref DESC"

type Store interface {
	Save(
		ctx context.Context,
		vm ViewModel,
	) error
	SaveVersion(
		ctx context.Context,
		ns domain.Namespace,
		arrow domain.Arrow,
	) error
	Delete(
		ctx context.Context,
		ns string,
	) error
	FindByKey(
		ctx context.Context,
		ns string,
	) (*ViewModel, error)
	FindAll(
		ctx context.Context,
	) ([]ViewModel, error)
}

type storageService struct {
	db *gormdb.DB
}

func New(
	db *gormdb.DB,
) (Store, error) {
	if err := newSchema(db); err != nil {
		return nil, err
	}
	return &storageService{db: db}, nil
}

// Save replaces the whole aggregate for a bare namespace: the version rows it
// is given become the only ones, and every derived row is rebuilt from them.
func (s *storageService) Save(
	ctx context.Context,
	vm ViewModel,
) error {
	bare := vm.Namespace.BareNamespace().String()

	return s.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if err := clearVersions(tx, bare); err != nil {
			return err
		}
		for _, version := range vm.Versions {
			if err := writeVersion(tx, bare, version.Namespace, version.Metadata); err != nil {
				return err
			}
		}
		return refreshParent(tx, bare)
	})
}

// SaveVersion writes exactly one version row and rebuilds the parent from SQL,
// so concurrent writers for different refs of one namespace cannot lose an
// update the way a read-modify-write of the whole aggregate would.
func (s *storageService) SaveVersion(
	ctx context.Context,
	ns domain.Namespace,
	arrow domain.Arrow,
) error {
	bare := ns.BareNamespace().String()

	return s.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if err := writeVersion(tx, bare, ns, arrow); err != nil {
			return err
		}
		return refreshParent(tx, bare)
	})
}

func (s *storageService) Delete(
	ctx context.Context,
	ns string,
) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		return deleteNamespace(tx, ns)
	})
}

func (s *storageService) FindByKey(
	ctx context.Context,
	ns string,
) (*ViewModel, error) {
	var parents []arrowRow
	if err := s.db.WithContext(ctx).
		Where("namespace = ?", ns).
		Find(&parents).Error; err != nil {
		return nil, fmt.Errorf("storage: find: %w", err)
	}
	if len(parents) == 0 {
		return nil, nil
	}

	versions, err := s.loadVersions(ctx, []string{ns})
	if err != nil {
		return nil, err
	}

	vms := assemble(parents, versions)
	return &vms[0], nil
}

func (s *storageService) FindAll(
	ctx context.Context,
) ([]ViewModel, error) {
	var parents []arrowRow
	if err := s.db.WithContext(ctx).
		Order("namespace").
		Find(&parents).Error; err != nil {
		return nil, fmt.Errorf("storage: find all: %w", err)
	}

	versions, err := s.loadVersions(ctx, nil)
	if err != nil {
		return nil, err
	}

	return assemble(parents, versions), nil
}

// loadVersions groups every version row by bare namespace, preferred first.
// A nil namespaces slice loads the whole table.
func (s *storageService) loadVersions(
	ctx context.Context,
	namespaces []string,
) (map[string][]VersionRef, error) {
	q := s.db.WithContext(ctx).Order("namespace").Order(versionOrder)
	if namespaces != nil {
		q = q.Where("namespace IN ?", namespaces)
	}

	var rows []arrowVersionRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("storage: load versions: %w", err)
	}

	grouped := make(map[string][]VersionRef, len(rows))
	for _, row := range rows {
		arrow, err := unmarshalArrow(row.Manifest)
		if err != nil {
			return nil, err
		}
		grouped[row.Namespace] = append(grouped[row.Namespace], VersionRef{
			Namespace: arrow.Namespace,
			Metadata:  *arrow,
		})
	}
	return grouped, nil
}

func assemble(
	parents []arrowRow,
	versions map[string][]VersionRef,
) []ViewModel {
	result := make([]ViewModel, 0, len(parents))
	for _, parent := range parents {
		vm := ViewModel{
			Namespace:  domain.Namespace(parent.Namespace),
			Versions:   versions[parent.Namespace],
			Provenance: parent.Provenance,
		}
		if vm.Versions == nil {
			vm.Versions = []VersionRef{}
		}
		if len(vm.Versions) > 0 {
			vm.Metadata = vm.Versions[0].Metadata
		}
		result = append(result, vm)
	}
	return result
}

func clearVersions(
	tx *gormdb.DB,
	bare string,
) error {
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowVersionRow{}).Error; err != nil {
		return fmt.Errorf("storage: clear versions: %w", err)
	}
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowOSRow{}).Error; err != nil {
		return fmt.Errorf("storage: clear os: %w", err)
	}
	return nil
}

func writeVersion(
	tx *gormdb.DB,
	bare string,
	ns domain.Namespace,
	arrow domain.Arrow,
) error {
	manifest, err := json.Marshal(arrow)
	if err != nil {
		return fmt.Errorf("storage: marshal manifest: %w", err)
	}

	ref := ns.Ref()
	row := arrowVersionRow{
		Namespace:     bare,
		Ref:           ref,
		InstalledRef:  arrow.InstalledRef,
		InstalledAt:   arrow.InstalledAt.Unix(),
		UserInstalled: arrow.UserInstalled,
		Manifest:      manifest,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "ref"}},
		UpdateAll: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("storage: upsert version: %w", err)
	}

	return replaceOS(tx, bare, ref, arrow.Targets)
}

func replaceOS(
	tx *gormdb.DB,
	bare string,
	ref string,
	targets map[domain.OS]domain.Target,
) error {
	if err := tx.Where("namespace = ? AND ref = ?", bare, ref).
		Delete(&arrowOSRow{}).Error; err != nil {
		return fmt.Errorf("storage: clear os: %w", err)
	}
	for _, os := range sortedKeys(targets) {
		if err := tx.Create(&arrowOSRow{
			Namespace: bare,
			Ref:       ref,
			OS:        string(os),
		}).Error; err != nil {
			return fmt.Errorf("storage: write os: %w", err)
		}
	}
	return nil
}

// refreshParent recomputes the parent row, its tags and its FTS entry from the
// version rows, so aggregation happens in SQL rather than in the caller.
func refreshParent(
	tx *gormdb.DB,
	bare string,
) error {
	var rows []arrowVersionRow
	if err := tx.Where("namespace = ?", bare).
		Order(versionOrder).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("storage: load versions: %w", err)
	}

	var preferred domain.Arrow
	if len(rows) > 0 {
		arrow, err := unmarshalArrow(rows[0].Manifest)
		if err != nil {
			return err
		}
		preferred = *arrow
	}

	parent := arrowRow{
		Namespace:     bare,
		Name:          preferred.Name,
		Description:   preferred.Description,
		License:       preferred.License,
		URL:           preferred.URL,
		Icon:          preferred.Media.Icon,
		Banner:        preferred.Media.Banner,
		Provenance:    provenanceOf(preferred),
		UserInstalled: preferred.UserInstalled,
		UpdatedAt:     latestInstalledAt(rows),
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}},
		UpdateAll: true,
	}).Create(&parent).Error; err != nil {
		return fmt.Errorf("storage: upsert arrow: %w", err)
	}

	if err := replaceTags(tx, bare, preferred.Tags); err != nil {
		return err
	}
	return replaceFTS(tx, bare, preferred)
}

func replaceTags(
	tx *gormdb.DB,
	bare string,
	tags []string,
) error {
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowTagRow{}).Error; err != nil {
		return fmt.Errorf("storage: clear tags: %w", err)
	}
	for _, tag := range dedupe(tags) {
		if err := tx.Create(&arrowTagRow{Namespace: bare, Tag: tag}).Error; err != nil {
			return fmt.Errorf("storage: write tag: %w", err)
		}
	}
	return nil
}

func replaceFTS(
	tx *gormdb.DB,
	bare string,
	arrow domain.Arrow,
) error {
	if err := tx.Exec(
		`DELETE FROM catalog_arrows_fts WHERE namespace = ?`, bare,
	).Error; err != nil {
		return fmt.Errorf("storage: clear fts: %w", err)
	}
	if err := tx.Exec(
		`INSERT INTO catalog_arrows_fts (namespace, name, description, tags)
		 VALUES (?, ?, ?, ?)`,
		bare, arrow.Name, arrow.Description, strings.Join(arrow.Tags, " "),
	).Error; err != nil {
		return fmt.Errorf("storage: write fts: %w", err)
	}
	return nil
}

func deleteNamespace(
	tx *gormdb.DB,
	bare string,
) error {
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowRow{}).Error; err != nil {
		return fmt.Errorf("storage: delete arrow: %w", err)
	}
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowVersionRow{}).Error; err != nil {
		return fmt.Errorf("storage: delete versions: %w", err)
	}
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowTagRow{}).Error; err != nil {
		return fmt.Errorf("storage: delete tags: %w", err)
	}
	if err := tx.Where("namespace = ?", bare).
		Delete(&arrowOSRow{}).Error; err != nil {
		return fmt.Errorf("storage: delete os: %w", err)
	}
	if err := tx.Exec(
		`DELETE FROM catalog_arrows_fts WHERE namespace = ?`, bare,
	).Error; err != nil {
		return fmt.Errorf("storage: delete fts: %w", err)
	}
	return nil
}

// provenanceOf covers the two values derivable from the arrow aggregate.
// ProvenanceCollection needs the collection read model and is not written here.
func provenanceOf(
	arrow domain.Arrow,
) string {
	if arrow.UserInstalled {
		return ProvenanceInstalled
	}
	return ProvenanceDependency
}

func latestInstalledAt(
	rows []arrowVersionRow,
) int64 {
	var latest int64
	for i, row := range rows {
		if i == 0 || row.InstalledAt > latest {
			latest = row.InstalledAt
		}
	}
	return latest
}

func sortedKeys(
	targets map[domain.OS]domain.Target,
) []domain.OS {
	keys := make([]domain.OS, 0, len(targets))
	for os := range targets {
		keys = append(keys, os)
	}
	slices.Sort(keys)
	return keys
}

func dedupe(
	values []string,
) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func unmarshalArrow(
	data []byte,
) (*domain.Arrow, error) {
	var arrow domain.Arrow
	if err := json.Unmarshal(data, &arrow); err != nil {
		return nil, fmt.Errorf("storage: unmarshal manifest: %w", err)
	}
	return &arrow, nil
}
