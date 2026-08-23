package dto

import (
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SearchResultDTO is one arrow matched by a search. Lane B streams this exact
// shape over the WebSocket, so every field has to make sense for an arrow that
// is merely discoverable, not only for one already on this machine: anything
// that can only be filled in for an installed arrow carries its zero value.
type SearchResultDTO struct {
	Namespace   string   `json:"namespace" yaml:"namespace"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Tags        []string `json:"tags" yaml:"tags"`
	// Media is the icon/banner pair declared by the manifest.
	Media domain.ArrowMedia `json:"media" yaml:"media"`
	// Versions are the refs known locally, not every ref that exists upstream.
	Versions []string `json:"versions" yaml:"versions"`
	// CompatibleOS is a denormalised projection of the last compile and is
	// advisory: install-time re-resolution is authoritative.
	CompatibleOS []string `json:"compatible_os" yaml:"compatible_os"`
	// Provenance is empty when the server cannot say: a streamed result the
	// catalog already holds is known to be yours, but discovery does not know
	// which provenance your catalog recorded for it.
	Provenance string `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	// Installed says the catalog holds the arrow, which is the only thing that
	// separates what you have from what you could have. It is narrower than
	// Known and never follows from it: an arrow can be on this machine because
	// a search once cached its manifest, which installs nothing.
	Installed bool `json:"installed" yaml:"installed"`
	// Known says the arrow is already on this machine by either route — the
	// catalog holds it, or the vault index cached its manifest. Every GET
	// /v0/search result is known by construction. A client merging streamed
	// results over an already-rendered list must keep its own row when this is
	// true, or it will overwrite a correct provenance with a less specific one.
	Known  bool   `json:"known" yaml:"known"`
	Stars  int    `json:"stars" yaml:"stars"`
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
}

func SearchResultDTOFrom(
	r models.SearchResult,
) SearchResultDTO {
	oses := make([]string, 0, len(r.CompatibleOS))
	for _, os := range r.CompatibleOS {
		oses = append(oses, string(os))
	}

	return SearchResultDTO{
		Namespace:    string(r.Namespace),
		Name:         r.Name,
		Description:  r.Description,
		Tags:         nonNil(r.Tags),
		Media:        r.Media,
		Versions:     nonNil(r.Versions),
		CompatibleOS: oses,
		Provenance:   r.Provenance,
		Installed:    r.Installed,
		Known:        r.Known,
		Stars:        r.Stars,
		Source:       r.Source,
	}
}

// SearchResultDTOFromDiscovery renders one streamed discovery result. It builds
// the same model Lane A builds and hands it to the same mapper, so the stream
// cannot drift from GET /v0/search into a second shape the client would have to
// render differently.
//
// Installed follows the catalog alone, never Known. Discovery indexes every
// arrow it proves, so each pass makes its own results vault-known to the next
// one; taking Installed from Known would report an arrow the user merely
// browsed last week as one they have, and installed is exactly the field the
// re-query relies on to tell those apart.
//
// An arrow the catalog holds is reported installed and carries no provenance:
// discovery knows only that the catalog has it, not whether it was installed
// directly, pulled in as a dependency, or came from a followed collection.
// Claiming "seen" there would be a downgrade a merging client cannot detect.
//
// Everything else carries the seen provenance and the single ref discovery
// verified — matching how Lane A renders a vault row. A followed collection
// caches its arrows into the vault without catalog rows, so such an arrow is
// streamed as seen where Lane A would say collection; known stays true, which
// is what tells a merging client to keep the better row it already has.
func SearchResultDTOFromDiscovery(
	r discovery.Result,
) SearchResultDTO {
	provenance := models.ProvenanceSeen
	if r.InCatalog {
		provenance = ""
	}

	return SearchResultDTOFrom(models.SearchResult{
		Namespace:    r.Namespace,
		Name:         r.Arrow.Name,
		Description:  r.Arrow.Description,
		Tags:         r.Arrow.Tags,
		Media:        r.Arrow.Media,
		Versions:     discoveredVersions(r.Arrow.Namespace.Ref()),
		CompatibleOS: discoveredOS(r.Arrow.Targets),
		Provenance:   provenance,
		Installed:    r.InCatalog,
		Known:        r.Known(),
		Stars:        r.Stars,
		Source:       r.Source,
	})
}

func discoveredVersions(
	ref string,
) []string {
	if ref == "" {
		return nil
	}
	return []string{ref}
}

func discoveredOS(
	targets map[domain.OS]domain.Target,
) []domain.OS {
	oses := make([]domain.OS, 0, len(targets))
	for os := range targets {
		oses = append(oses, os)
	}
	slices.Sort(oses)
	return oses
}

// nonNil keeps absent lists rendering as [] rather than null, so clients can
// iterate every result the same way.
func nonNil(
	values []string,
) []string {
	if values == nil {
		return []string{}
	}
	return values
}
