package vault

import (
	"encoding/json"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ManifestFile struct {
	Content  []byte
	Filename string // "ARROW.md" or "arrow.yaml"
	// Meta carries searchable metadata for the index. Nil means cache the
	// bytes without indexing them.
	Meta *IndexMeta
	// Ref and Commit name the release the manifest was read at, when the
	// writer knew them. A reader reuses the bytes for that release only: two
	// tags of one commit can publish different release assets.
	Ref    string
	Commit string
	// Default marks the manifest a refless namespace settled on: the entry a
	// later view of that namespace is answered from.
	Default bool
	// Channels is the channel list of the repository when the manifest was
	// filed, opaque to the vault. It is kept across a write that carries none.
	Channels []byte
	// DefaultAt is when Default was last set on the entry, so two marks can be
	// ordered; a write that does not set Default keeps it. A read fills it.
	DefaultAt time.Time
}

type CollectionVaultEntry struct {
	Collection *domain.Collection `json:"collection"`
	Metadata   VaultMetadata      `json:"metadata"`
}

type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	Filename string    `json:"filename"`
	// Namespace names the entry, since a capped filename cannot be decoded
	// back to it.
	Namespace domain.Namespace `json:"namespace,omitempty"`
	// NotFound marks this entry as a confirmed-absent result rather than a
	// cached manifest: no Filename, no manifest bytes on disk, just the
	// fact "this exact ref genuinely has no manifest", subject to the same
	// TTL as a positive entry. See ErrConfirmedAbsent.
	NotFound bool `json:"not_found,omitempty"`
	// Ref and Commit name the release the cached manifest was read at, empty
	// when the writer did not know them.
	Ref     string `json:"ref,omitempty"`
	Commit  string `json:"commit,omitempty"`
	Default bool   `json:"default,omitempty"`
	// DefaultAt is ManifestFile.DefaultAt.
	DefaultAt time.Time `json:"default_at,omitempty"`
	// Channels is ManifestFile.Channels.
	Channels json.RawMessage `json:"channels,omitempty"`
}
