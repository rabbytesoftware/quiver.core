package resolvers

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type ConstraintResolver interface {
	Resolve(ctx context.Context, ns domain.Namespace, pattern string) (string, error)
}

type constraintResolver struct {
	timeout time.Duration
}

func NewConstraintResolver(timeout time.Duration) ConstraintResolver {
	return &constraintResolver{timeout: timeout}
}

func (c *constraintResolver) Resolve(
	ctx context.Context,
	ns domain.Namespace,
	pattern string,
) (string, error) {
	return c.resolveWithCloneURL(ctx, ns.BareNamespace().CloneURL(), pattern)
}

func (c *constraintResolver) resolveWithCloneURL(
	ctx context.Context,
	cloneURL string,
	pattern string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{cloneURL},
	})

	refs, err := remote.ListContext(ctx, &gogit.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("constraint: list refs for %s: %w", cloneURL, err)
	}

	var matched []string
	for _, ref := range refs {
		name := ref.Name()
		if !name.IsTag() {
			continue
		}
		tagName := name.Short()
		ok, err := path.Match(pattern, tagName)
		if err != nil {
			return "", fmt.Errorf("constraint: invalid pattern %q: %w", pattern, err)
		}
		if ok {
			matched = append(matched, tagName)
		}
	}

	if len(matched) == 0 {
		return "", fmt.Errorf("constraint: no git tags match pattern %q for %s", pattern, cloneURL)
	}

	sortTagsDesc(matched)
	return matched[0], nil
}

// sortTagsDesc sorts tags in descending order. If all tags are valid semver,
// numeric comparison is used; otherwise lexicographic order.
func sortTagsDesc(tags []string) {
	allSemver := true
	for _, t := range tags {
		if !isSemver(t) {
			allSemver = false
			break
		}
	}

	if allSemver {
		sort.Slice(tags, func(i, j int) bool {
			return semverGT(tags[i], tags[j])
		})
		return
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i] > tags[j]
	})
}

func isSemver(tag string) bool {
	s := strings.TrimPrefix(tag, "v")
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return false
		}
	}
	return true
}

func semverGT(a, b string) bool {
	aParts := semverParts(a)
	bParts := semverParts(b)
	for i := range aParts {
		if aParts[i] != bParts[i] {
			return aParts[i] > bParts[i]
		}
	}
	return false
}

func semverParts(tag string) [3]int {
	s := strings.TrimPrefix(tag, "v")
	parts := strings.Split(s, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
