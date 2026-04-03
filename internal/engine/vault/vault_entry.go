package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// VaultEntry is the value returned by GetArrow.
// IndirectDependencies is nil before the arrow has been installed;
// it is populated by PutArrow after DepTree resolves the full graph.
type VaultEntry struct {
	Manifest             *domain.ArrowManifest `json:"manifest"`
	Metadata             VaultMetadata         `json:"metadata"`
	IndirectDependencies []domain.Namespace    `json:"indirect_dependencies,omitempty"`
}

// QuiverVaultEntry is the value returned by GetQuiver.
type QuiverVaultEntry struct {
	Manifest *domain.QuiverManifest `json:"manifest"`
	Metadata VaultMetadata          `json:"metadata"`
}

// VaultMetadata records when and how a manifest was cached.
type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
	OS       string    `json:"os"`
}
