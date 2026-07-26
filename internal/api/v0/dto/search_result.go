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
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	// Media is the icon/banner pair declared by the manifest.
	Media domain.ArrowMedia `json:"media"`
	// Versions are the refs known locally, not every ref that exists upstream.
	Versions []string `json:"versions"`
	// CompatibleOS is a denormalised projection of the last compile and is
	// advisory: install-time re-resolution is authoritative.
	CompatibleOS []string `json:"compatible_os"`
	Provenance   string   `json:"provenance"`
	Installed    bool     `json:"installed"`
	Stars        int      `json:"stars"`
	Source       string   `json:"source,omitempty"`
}

func SearchResultDTOFrom(
	r models.SearchResult,
) SearchResultDTO {
	oses := make([]string, 0, len(r.CompatibleOS))
	for _, os := range r.CompatibleOS {
		oses = append(oses, string(os))
	}

	return SearchResultDTO{
		Namespace:   string(r.Namespace),
		Name:        r.Name,
		Description: r.Description,
		Tags:        nonNil(r.Tags),
		Media: domain.ArrowMedia{
			Icon:   r.Icon,
			Banner: r.Banner,
		},
		Versions:     nonNil(r.Versions),
		CompatibleOS: oses,
		Provenance:   r.Provenance,
		Installed:    r.Installed,
		Stars:        r.Stars,
		Source:       r.Source,
	}
}

// SearchResultDTOFromDiscovery renders one streamed discovery result. It builds
// the same model Lane A builds and hands it to the same mapper, so the stream
// cannot drift from GET /v0/search into a second shape the client would have to
// render differently.
//
// A discovered arrow is never installed and its manifest was proven just now,
// so it carries the seen provenance and the single ref discovery verified.
func SearchResultDTOFromDiscovery(
	r discovery.Result,
) SearchResultDTO {
	return SearchResultDTOFrom(models.SearchResult{
		Namespace:    r.Namespace,
		Name:         r.Arrow.Name,
		Description:  r.Arrow.Description,
		Tags:         r.Arrow.Tags,
		Icon:         r.Arrow.Media.Icon,
		Banner:       r.Arrow.Media.Banner,
		Versions:     discoveredVersions(r.Arrow.Version),
		CompatibleOS: discoveredOS(r.Arrow.Targets),
		Provenance:   models.ProvenanceSeen,
		Stars:        r.Stars,
		Source:       r.Source,
	})
}

func discoveredVersions(
	version string,
) []string {
	if version == "" {
		return nil
	}
	return []string{version}
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
