package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ManifestFile struct {
	Content  []byte
	Filename string // "ARROW.md" or "arrow.yaml"
}

type CollectionVaultEntry struct {
	Collection *domain.Collection `json:"collection"`
	Metadata   VaultMetadata      `json:"metadata"`
}

type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
}
