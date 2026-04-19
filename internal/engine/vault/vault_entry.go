package vault

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// VaultEntry is the value returned by GetArrow.
type VaultEntry struct {
	Manifest domain.Arrow  `json:"manifest"`
	Metadata VaultMetadata `json:"metadata"`
}

// QuiverVaultEntry is the value returned by GetQuiver.
type QuiverVaultEntry struct {
	Manifest *domain.QuiverManifest `json:"manifest"`
	Metadata VaultMetadata          `json:"metadata"`
}

// VaultMetadata records when a manifest was cached.
type VaultMetadata struct {
	CachedAt time.Time `json:"cached_at"`
}
