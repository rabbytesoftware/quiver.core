package store

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// ArrowViewModel is the aggregated read model for a bare namespace,
// grouping all versions of the same package.
type ArrowViewModel struct {
	Namespace domain.Namespace

	// Metadata is the metadata for the preferred version
	// (user-installed if one exists, else latest by InstalledAt).
	// Contains the full Arrow with ArrowMeta + installation fields.
	Metadata domain.Arrow

	Versions []ArrowVersionRef
}

// ArrowVersionRef is a single version entry within the aggregated view model.
type ArrowVersionRef struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
}
