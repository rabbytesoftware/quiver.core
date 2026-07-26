package storage

// arrowRow is the parent row for a bare namespace. Every column is derived from
// the preferred version row, so the catalog can be matched, ranked and rendered
// without opening a manifest blob.
type arrowRow struct {
	Namespace     string `gorm:"primaryKey;column:namespace"`
	Name          string `gorm:"column:name"`
	Description   string `gorm:"column:description"`
	License       string `gorm:"column:license"`
	URL           string `gorm:"column:url"`
	Icon          string `gorm:"column:icon"`
	Banner        string `gorm:"column:banner"`
	Provenance    string `gorm:"column:provenance"`
	UserInstalled bool   `gorm:"column:user_installed"`
	UpdatedAt     int64  `gorm:"column:updated_at"`
}

func (arrowRow) TableName() string { return "catalog_arrows" }

// arrowVersionRow is one namespace@ref. Manifest holds the whole domain.Arrow as
// JSON: compiled targets are deeply nested and never queried, so normalising
// them would be a large schema for no query benefit.
type arrowVersionRow struct {
	Namespace     string `gorm:"primaryKey;column:namespace"`
	Ref           string `gorm:"primaryKey;column:ref"`
	InstalledRef  string `gorm:"column:installed_ref"`
	InstalledAt   int64  `gorm:"column:installed_at"`
	UserInstalled bool   `gorm:"column:user_installed"`
	Manifest      []byte `gorm:"column:manifest"`
}

func (arrowVersionRow) TableName() string { return "catalog_arrow_versions" }

type arrowTagRow struct {
	Namespace string `gorm:"primaryKey;column:namespace"`
	Tag       string `gorm:"primaryKey;column:tag"`
}

func (arrowTagRow) TableName() string { return "catalog_arrow_tags" }

type arrowOSRow struct {
	Namespace string `gorm:"primaryKey;column:namespace"`
	Ref       string `gorm:"primaryKey;column:ref"`
	OS        string `gorm:"primaryKey;column:os"`
}

func (arrowOSRow) TableName() string { return "catalog_arrow_os" }
