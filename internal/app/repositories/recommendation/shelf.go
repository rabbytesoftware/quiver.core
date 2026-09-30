package recommendation

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// Shelf is one named list of arrows as last refreshed. RefreshedAt is the zero
// time for a shelf that has never been filled.
type Shelf struct {
	ID          string
	Title       string
	RefreshedAt time.Time
	Entries     []Entry
}

// Entry is one arrow on a shelf, with what the vault index knows about it.
// Rows holds one row per known ref, best first; InCatalog says the arrow is
// already on this machine's catalog.
type Entry struct {
	Namespace domain.Namespace
	Stars     int
	Source    string
	Rows      []vault.IndexRow
	InCatalog bool
}
