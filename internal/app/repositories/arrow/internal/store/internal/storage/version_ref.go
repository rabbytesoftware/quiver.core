package storage

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type VersionRef struct {
	Namespace domain.Namespace
	Metadata  domain.Arrow
	// LastVersionCheckAt lives outside Metadata's JSON blob — see
	// arrowVersionRow — so a caller already holding a VersionRef from a plain
	// read can decide version-check staleness without a second query.
	LastVersionCheckAt time.Time
}
