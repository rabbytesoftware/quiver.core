package resolve

import (
	"sort"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/fsguard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/models"
)

func pickDesktop(
	found []models.Candidate,
	repo string,
	entryName string,
) (models.Candidate, string) {
	if len(found) == 0 {
		return models.Candidate{}, models.ReasonNoDesktop
	}
	if len(found) == 1 {
		return found[0], ""
	}

	matches := matching(found, repo, entryName)
	if len(matches) == 0 {
		return models.Candidate{}, models.ReasonAmbiguous
	}
	return matches[0], ""
}

func renamed(
	c models.Candidate,
	name string,
) models.Candidate {
	if fsguard.SafeName(name) {
		c.Name = name
	}
	return c
}

func matching(
	found []models.Candidate,
	repo string,
	entryName string,
) []models.Candidate {
	matches := []models.Candidate{}
	for _, c := range found {
		if strings.EqualFold(c.Name, repo) || (entryName != "" && strings.EqualFold(c.Name, entryName)) {
			matches = append(matches, c)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Depth < matches[j].Depth })
	return matches
}

func shallowest(
	found []models.Candidate,
) []models.Candidate {
	depth := found[0].Depth
	for _, c := range found {
		depth = min(depth, c.Depth)
	}

	top := []models.Candidate{}
	for _, c := range found {
		if c.Depth == depth {
			top = append(top, c)
		}
	}
	return top
}

func uniqueNames(
	candidates []models.Candidate,
) []models.Candidate {
	seen := map[string]bool{}
	unique := []models.Candidate{}
	for _, c := range candidates {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		unique = append(unique, c)
	}
	return unique
}

func repoName(
	bare domain.Namespace,
) string {
	parts := strings.Split(string(bare), domain.NamespaceSeparator)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}
