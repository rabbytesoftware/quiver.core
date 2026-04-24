package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// ManifestFile is the raw manifest content as fetched from the remote source.
type ManifestFile struct {
	Content  []byte
	Filename string // "ARROW.md" or "arrow.yaml"
}

// QuiverVaultEntry is the value returned by GetQuiver. Unchanged.
type QuiverVaultEntry struct {
	Manifest *domain.QuiverManifest `json:"manifest"`
	Metadata VaultMetadata          `json:"metadata"`
}

// VaultMetadata records when a manifest was cached and its original filename.
type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
}
