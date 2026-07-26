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
type ViewModel struct {
	Namespace  domain.Namespace
	Metadata   domain.Arrow
	Versions   []VersionRef
	Provenance string
}
