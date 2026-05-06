package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ManifestFile struct {
	Content  []byte
	Filename string // "ARROW.md" or "arrow.yaml"
}

type QuiverVaultEntry struct {
	Quiver   *domain.Quiver `json:"quiver"`
	Metadata VaultMetadata  `json:"metadata"`
}

type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
}
