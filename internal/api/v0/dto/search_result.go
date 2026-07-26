package dto

import (
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
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
