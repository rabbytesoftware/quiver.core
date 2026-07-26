package discovery

import "github.com/rabbytesoftware/quiver.core/internal/domain"

// Result is one arrow proven to exist: its manifest was fetched and parsed, not
// merely advertised by a topic. Known says the arrow is already in the catalog
// or the vault index, so the client can dedup against what it already rendered
// rather than being denied the result.
type Result struct {
	Arrow     domain.Arrow
	Namespace domain.Namespace
	Stars     int
	Source    string
	Known     bool
}
