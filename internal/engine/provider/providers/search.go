package providers

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SearchRequest is one query against one host. Topics are the discovery
// markers a repository must carry; hosts intersect them rather than union
// them. Limit caps the number of candidates returned.
type SearchRequest struct {
	Text   string
	Topics []string
	Limit  int
}

// searchEachTopic runs one request per marker and unions the results.
//
// Both hosts intersect a multi-topic query: GitHub ANDs repeated topic
// qualifiers, GitLab ANDs a comma separated list. A union is what a marker
// list means — adding a marker must widen the set, never narrow it — so the
// fan-out happens here, one metered request per topic.
//
// The cost is real: N topics spend N units of the search budget. With the
// single default marker it is one request, identical to a plain query.
func searchEachTopic(
	ctx context.Context,
	topics []string,
	limit int,
	once func(ctx context.Context, topic string) ([]Candidate, error),
) ([]Candidate, error) {
	if len(topics) == 0 {
		return once(ctx, "")
	}

	seen := make(map[domain.Namespace]struct{})
	union := make([]Candidate, 0, limit)

	for _, topic := range topics {
		found, err := once(ctx, topic)
		if err != nil {
			return nil, err
		}
		for _, candidate := range found {
			if _, dup := seen[candidate.Namespace]; dup {
				continue
			}
			seen[candidate.Namespace] = struct{}{}
			union = append(union, candidate)
		}
	}
	return truncate(union, limit), nil
}
