package discovery

import "github.com/rabbytesoftware/quiver.core/internal/domain"

// Result is one arrow proven to exist: its manifest was fetched and parsed, not
// merely advertised by a topic.
//
// InCatalog and InVault are two separate facts because they answer two separate
// questions, and one flag answering both is what let a merely-browsed arrow
// report itself as installed. Neither is derived from the other: both stores are
// asked, so the pair cannot describe a state the machine is not in.
type Result struct {
	Arrow     domain.Arrow
	Namespace domain.Namespace
	Stars     int
	Source    string
	// InCatalog says the catalog holds this arrow — installed directly, pulled
	// in as a dependency, or brought in by a followed collection. It is the only
	// signal that the user has the arrow rather than could have it.
	InCatalog bool
	// InVault says the vault index already held this arrow when the pass looked,
	// which means an earlier pass proved it or a followed collection cached it.
	// Having browsed an arrow is not having it: this never implies InCatalog.
	InVault bool
}

// Known reports that the arrow is already on this machine by either route, so a
// client can dedup against what it has already rendered rather than being
// denied the result. It is derived rather than stored: a third field would have
// to be kept in step with these two, and drifting is the bug this shape exists
// to prevent.
func (r Result) Known() bool {
	return r.InCatalog || r.InVault
}
