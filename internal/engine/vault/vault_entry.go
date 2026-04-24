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
	Manifest *domain.QuiverManifest `json:"manifest"`
	Metadata VaultMetadata          `json:"metadata"`
}

type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
}
