package recommendation

import (
	"fmt"
	"log/slog"

	recommendationinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal"
)

// buildShelves turns the configured shelves into ones a refresh can run,
// dropping any that cannot: one typo in a shelf costs that shelf, not the list.
func buildShelves(
	configured []ShelfConfig,
) []recommendationinternal.Shelf {
	shelves := make([]recommendationinternal.Shelf, 0, len(configured))
	seen := make(map[string]struct{}, len(configured))

	for _, cfg := range configured {
		shelf, err := buildShelf(cfg, seen)
		if err != nil {
			slog.Warn("recommendation: skipping shelf", "shelf", cfg.ID, "err", err)
			continue
		}
		seen[shelf.ID] = struct{}{}
		shelves = append(shelves, shelf)
	}
	return shelves
}

func buildShelf(
	cfg ShelfConfig,
	seen map[string]struct{},
) (recommendationinternal.Shelf, error) {
	if cfg.ID == "" {
		return recommendationinternal.Shelf{}, fmt.Errorf("shelf has no id")
	}
	if _, dup := seen[cfg.ID]; dup {
		return recommendationinternal.Shelf{}, fmt.Errorf("shelf id is used twice")
	}
	if cfg.Limit < 1 {
		return recommendationinternal.Shelf{}, fmt.Errorf("limit must be at least 1, got %d", cfg.Limit)
	}
	if len(cfg.Sources) == 0 {
		return recommendationinternal.Shelf{}, fmt.Errorf("shelf has no sources")
	}

	sources := make([]recommendationinternal.Source, 0, len(cfg.Sources))
	for _, raw := range cfg.Sources {
		source, err := buildSource(raw)
		if err != nil {
			return recommendationinternal.Shelf{}, err
		}
		sources = append(sources, source)
	}
	return recommendationinternal.Shelf{ID: cfg.ID, Title: cfg.Title, Limit: cfg.Limit, Sources: sources}, nil
}

func buildSource(
	cfg SourceConfig,
) (recommendationinternal.Source, error) {
	if cfg.Host == "" {
		return recommendationinternal.Source{}, fmt.Errorf("source has no host")
	}
	if cfg.Sort != "" && cfg.Sort != "stars" && cfg.Sort != "updated" {
		return recommendationinternal.Source{}, fmt.Errorf("sort must be stars or updated, got %q", cfg.Sort)
	}
	if cfg.MinStars < 0 {
		return recommendationinternal.Source{}, fmt.Errorf("min_stars must not be negative, got %d", cfg.MinStars)
	}
	if cfg.MaxStars < 0 {
		return recommendationinternal.Source{}, fmt.Errorf("max_stars must not be negative, got %d", cfg.MaxStars)
	}
	if cfg.MaxStars > 0 && cfg.MaxStars < cfg.MinStars {
		return recommendationinternal.Source{}, fmt.Errorf("max_stars %d is below min_stars %d", cfg.MaxStars, cfg.MinStars)
	}

	window, err := parseWindow(cfg.PushedWithin)
	if err != nil {
		return recommendationinternal.Source{}, err
	}
	return recommendationinternal.Source{
		Host:         cfg.Host,
		Sort:         cfg.Sort,
		MinStars:     cfg.MinStars,
		MaxStars:     cfg.MaxStars,
		PushedWithin: window,
	}, nil
}
