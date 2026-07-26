package models

import "github.com/rabbytesoftware/quiver.core/internal/domain"

// Provenance records why an arrow is known to this machine. The first three
// mirror the values the catalog read model stores; ProvenanceSeen is assigned
// to arrows that exist only in the vault index.
const (
	ProvenanceInstalled  = "installed"
	ProvenanceDependency = "dependency"
	ProvenanceCollection = "collection"
	ProvenanceSeen       = "seen"
)

// SearchQuery selects arrows by free text, optionally constrained to a
// platform. It crosses the repository boundary, so it repeats the storage
// query shape rather than exposing the storage package.
type SearchQuery struct {
	Text  string
	OS    domain.OS
	Limit int
}

// CatalogHit is one catalog search match: the preferred manifest for a bare
// namespace, every ref known for it, and why it is in the catalog.
type CatalogHit struct {
	Namespace  domain.Namespace
	Metadata   domain.Arrow
	Refs       []string
	Provenance string
}

// SearchResult is one merged, ranked arrow. Installed, Provenance, Versions
// and CompatibleOS describe what can be done with the arrow; they never
// contribute to its relevance score.
type SearchResult struct {
	Namespace    domain.Namespace
	Name         string
	Description  string
	Tags         []string
	Icon         string
	Banner       string
	Versions     []string
	CompatibleOS []domain.OS
	Provenance   string
	Installed    bool
	Stars        int
	Source       string
}
