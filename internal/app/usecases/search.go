package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	collectionrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

const defaultSearchLimit = 25

// SearchUsecase answers offline: it reads the catalog and the vault index and
// never touches the network.
type SearchUsecase interface {
	Search(
		ctx context.Context,
		q models.SearchQuery,
	) ([]models.SearchResult, error)
}

type searchUsecase struct {
	arrows      arrowrepo.Arrow
	vault       vault.Vault
	collections collectionrepo.Collection
}

func NewSearchUsecase(
	arrows arrowrepo.Arrow,
	v vault.Vault,
	collections collectionrepo.Collection,
) SearchUsecase {
	return &searchUsecase{
		arrows:      arrows,
		vault:       v,
		collections: collections,
	}
}

func (s *searchUsecase) Search(
	ctx context.Context,
	q models.SearchQuery,
) ([]models.SearchResult, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		// The handler rejects this with a 400 before ever getting here; this is
		// the guard for any other caller.
		return nil, fmt.Errorf("search: query text is empty")
	}

	limit := q.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	hits, err := s.arrows.Search(ctx, models.SearchQuery{Text: text, OS: q.OS, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("search: catalog: %w", err)
	}

	rows, err := s.vault.SearchArrows(ctx, vault.IndexQuery{Text: text, OS: q.OS, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("search: vault: %w", err)
	}

	followed := s.followedArrows(ctx)
	catalog := catalogResults(hits, followed)
	seen := seenResults(rows, namespaceSet(catalog), followed)

	merged := merge(text, catalog, seen, followed)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// followedArrows collects the bare namespace of every arrow in a followed
// collection. Losing the set costs one ranking signal; it must never cost the
// search, so a read failure is logged and treated as "nothing is followed".
func (s *searchUsecase) followedArrows(
	ctx context.Context,
) map[domain.Namespace]struct{} {
	set := make(map[domain.Namespace]struct{})

	collections, err := s.collections.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "search: list followed collections failed", "err", err)
		return set
	}

	for _, collection := range collections {
		for _, arrow := range collection.Arrows {
			set[arrow.Namespace.BareNamespace()] = struct{}{}
		}
	}
	return set
}

func catalogResults(
	hits []models.CatalogHit,
	followed map[domain.Namespace]struct{},
) []models.SearchResult {
	results := make([]models.SearchResult, 0, len(hits))
	for _, hit := range hits {
		bare := hit.Namespace.BareNamespace()

		provenance := hit.Provenance
		if _, ok := followed[bare]; ok {
			provenance = models.ProvenanceCollection
		}

		results = append(results, models.SearchResult{
			Namespace:    bare,
			Name:         hit.Metadata.Name,
			Description:  hit.Metadata.Description,
			Tags:         hit.Metadata.Tags,
			Icon:         hit.Metadata.Media.Icon,
			Banner:       hit.Metadata.Media.Banner,
			Versions:     sortedRefs(hit.Refs),
			CompatibleOS: targetOS(hit.Metadata.Targets),
			Provenance:   provenance,
			Installed:    true,
		})
	}
	return results
}

// seenResults collapses the per-ref vault rows onto their bare namespace,
// keeping the highest-ranked row's metadata. A namespace the catalog already
// answered for is dropped outright: the catalog row wins the collision and the
// vault row contributes nothing, not even its refs.
func seenResults(
	rows []vault.IndexRow,
	catalog map[domain.Namespace]struct{},
	followed map[domain.Namespace]struct{},
) []models.SearchResult {
	order := make([]domain.Namespace, 0, len(rows))
	grouped := make(map[domain.Namespace]*models.SearchResult, len(rows))

	for _, row := range rows {
		bare := row.Namespace.BareNamespace()
		if _, clash := catalog[bare]; clash {
			continue
		}
		if existing, ok := grouped[bare]; ok {
			existing.Versions = append(existing.Versions, row.Ref)
			continue
		}
		result := seenResult(bare, row, followed)
		grouped[bare] = &result
		order = append(order, bare)
	}

	results := make([]models.SearchResult, 0, len(order))
	for _, ns := range order {
		result := grouped[ns]
		result.Versions = sortedRefs(result.Versions)
		results = append(results, *result)
	}
	return results
}

// seenResult builds a vault-sourced result. Following a collection caches its
// arrows into the vault without adding catalog rows, so membership of a followed
// collection is the only thing that distinguishes a curated arrow from one
// Quiver merely encountered — it cannot be read off the catalog.
func seenResult(
	bare domain.Namespace,
	row vault.IndexRow,
	followed map[domain.Namespace]struct{},
) models.SearchResult {
	provenance := models.ProvenanceSeen
	if _, ok := followed[bare]; ok {
		provenance = models.ProvenanceCollection
	}

	return models.SearchResult{
		Namespace:    bare,
		Name:         row.Meta.Arrow.Name,
		Description:  row.Meta.Arrow.Description,
		Tags:         row.Meta.Arrow.Tags,
		Icon:         row.Meta.Arrow.Media.Icon,
		Banner:       row.Meta.Arrow.Media.Banner,
		Versions:     []string{row.Ref},
		CompatibleOS: row.Meta.OS,
		Provenance:   provenance,
		Stars:        row.Meta.Stars,
		Source:       row.Meta.Source,
	}
}

// merge scores each source against its own result set — bm25 is corpus-relative,
// so the two are not comparable until each has been normalised — and then
// orders the union.
func merge(
	query string,
	catalog []models.SearchResult,
	seen []models.SearchResult,
	followed map[domain.Namespace]struct{},
) []models.SearchResult {
	items := make([]scored, 0, len(catalog)+len(seen))
	items = append(items, scoreSet(query, catalog, followed)...)
	// Curation applies to vault rows too: Follow caches a collection's arrows
	// into the vault without creating catalog rows, so restricting the boost to
	// catalog hits would make it dead code in exactly the case it exists for.
	items = append(items, scoreSet(query, seen, followed)...)
	sortScored(items)

	results := make([]models.SearchResult, 0, len(items))
	for _, item := range items {
		results = append(results, item.result)
	}
	return results
}

func scoreSet(
	query string,
	results []models.SearchResult,
	followed map[domain.Namespace]struct{},
) []scored {
	bases := normalise(positionScores(len(results)))

	items := make([]scored, 0, len(results))
	for i, result := range results {
		_, isFollowed := followed[result.Namespace]
		items = append(items, scored{
			result: result,
			score:  applyBoosts(bases[i], result, query, isFollowed),
		})
	}
	return items
}

func namespaceSet(
	results []models.SearchResult,
) map[domain.Namespace]struct{} {
	set := make(map[domain.Namespace]struct{}, len(results))
	for _, result := range results {
		set[result.Namespace] = struct{}{}
	}
	return set
}

func sortedRefs(
	refs []string,
) []string {
	out := make([]string, len(refs))
	copy(out, refs)
	slices.Sort(out)
	return slices.Compact(out)
}

func targetOS(
	targets map[domain.OS]domain.Target,
) []domain.OS {
	oses := make([]domain.OS, 0, len(targets))
	for os := range targets {
		oses = append(oses, os)
	}
	slices.Sort(oses)
	return oses
}
