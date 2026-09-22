package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ManifestFile struct {
	Content  []byte
	Filename string // "ARROW.md" or "arrow.yaml"
	// Meta carries searchable metadata for the index. Nil means cache the
	// bytes without indexing them.
	Meta *IndexMeta
}

type CollectionVaultEntry struct {
	Collection *domain.Collection `json:"collection"`
	Metadata   VaultMetadata      `json:"metadata"`
}

type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
	// NotFound marks this entry as a confirmed-absent result rather than a
	// cached manifest: no Filename, no manifest bytes on disk, just the
	// fact "this exact ref genuinely has no manifest", subject to the same
	// TTL as a positive entry. See ErrConfirmedAbsent.
	NotFound bool `json:"not_found,omitempty"`
}
