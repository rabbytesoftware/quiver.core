package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// IndexMeta is the searchable metadata a caller supplies alongside a manifest.
// Vault cannot parse manifests itself, so this must come from whoever already did.
type IndexMeta struct {
	Arrow  domain.ArrowMeta
	OS     []domain.OS
	Stars  int
	Source string
	Branch string
}

// IndexRow is one cached namespace@ref as returned by a search.
type IndexRow struct {
	Namespace domain.Namespace
	Ref       string
	Meta      IndexMeta
	SeenAt    time.Time
}

// IndexQuery selects rows by free text, optionally constrained to a platform.
type IndexQuery struct {
	Text  string
	OS    domain.OS
	Limit int
}

type arrowIndexRow struct {
	Namespace   string `gorm:"primaryKey;column:namespace"`
	Ref         string `gorm:"primaryKey;column:ref"`
	Name        string `gorm:"column:name"`
	Description string `gorm:"column:description"`
	Icon        string `gorm:"column:icon"`
	Banner      string `gorm:"column:banner"`
	Stars       int    `gorm:"column:stars"`
	Source      string `gorm:"column:source"`
	Filename    string `gorm:"column:filename"`
	Branch      string `gorm:"column:branch"`
	SeenAt      int64  `gorm:"column:seen_at"`
	RowExpireAt int64  `gorm:"column:row_expire_at"`
}

func (arrowIndexRow) TableName() string { return "vault_arrows" }

type arrowTagRow struct {
	Namespace string `gorm:"primaryKey;column:namespace"`
	Ref       string `gorm:"primaryKey;column:ref"`
	Tag       string `gorm:"primaryKey;column:tag"`
}

func (arrowTagRow) TableName() string { return "vault_arrow_tags" }

type arrowOSRow struct {
	Namespace string `gorm:"primaryKey;column:namespace"`
	Ref       string `gorm:"primaryKey;column:ref"`
	OS        string `gorm:"primaryKey;column:os"`
}

func (arrowOSRow) TableName() string { return "vault_arrow_os" }
