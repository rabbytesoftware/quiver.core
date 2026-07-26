package storage

import "github.com/rabbytesoftware/quiver.core/internal/domain"

// Provenance records why an arrow is in the catalog.
const (
	ProvenanceInstalled  = "installed"
	ProvenanceDependency = "dependency"
	ProvenanceCollection = "collection"
)

// ViewModel is the aggregated read model for a bare namespace,
// grouping all installed versions of the same package.
//
// Metadata and Provenance are derived from Versions on write and are
// populated on read: setting them before a Save has no effect. Deriving
// rather than storing them is what keeps search results, which read the
// parent row, from disagreeing with FindByKey, which rebuilds from the
// version rows.
type ViewModel struct {
	Namespace  domain.Namespace
	Metadata   domain.Arrow
	Versions   []VersionRef
	Provenance string
}
