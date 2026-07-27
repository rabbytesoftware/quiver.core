package providers

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Candidate is one repository carrying a discovery marker. Whether it really
// is an arrow is decided later, by fetching and parsing its manifest.
type Candidate struct {
	Namespace domain.Namespace
	Name      string
	// Description is whatever the host holds, which is not the manifest
	// description and may disagree with it.
	Description string
	Stars       int
	// Source is the host that answered, e.g. "github.com".
	Source string
	// DefaultBranch comes off the search response and is never guessed, so a
	// manifest fetch costs no extra request to discover it.
	DefaultBranch string
}

func candidateOf(
	host string,
	path string,
	name string,
	description string,
	stars int,
	branch string,
) (Candidate, bool) {
	ns := domain.Namespace(host + domain.NamespaceSeparator + path)
	if ns.Validate() != nil || ns.IsQuiverHosted() {
		return Candidate{}, false
	}

	return Candidate{
		Namespace:     ns,
		Name:          name,
		Description:   description,
		Stars:         stars,
		Source:        host,
		DefaultBranch: branch,
	}, true
}

func truncate(
	candidates []Candidate,
	limit int,
) []Candidate {
	if limit > 0 && len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}
