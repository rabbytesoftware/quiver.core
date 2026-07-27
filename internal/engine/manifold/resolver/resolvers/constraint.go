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
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ConstraintResolver answers the questions a remote's ref advertisement can
// answer: which tag satisfies a constraint, and which branch is the default.
type ConstraintResolver interface {
	Resolve(ctx context.Context, ns domain.Namespace, pattern string) (string, error)

	// DefaultBranch reports the branch the remote's HEAD points at. It is the
	// repository's real default branch on any git host, whatever it is named.
	DefaultBranch(ctx context.Context, ns domain.Namespace) (string, error)
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

func (c *constraintResolver) DefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	return c.defaultBranchWithCloneURL(ctx, ns.BareNamespace().CloneURL())
}

func (c *constraintResolver) defaultBranchWithCloneURL(
	ctx context.Context,
	cloneURL string,
) (string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
	if err != nil {
		return "", fmt.Errorf("default branch: list refs for %s: %w", cloneURL, err)
	}
	return headBranch(refs, cloneURL)
}

// headBranch reads the branch a remote's HEAD points at. The ref advertisement
// carries HEAD as a symbolic reference, so its target names the default branch
// without any host-specific API and without guessing from a list.
func headBranch(
	refs []*plumbing.Reference,
	cloneURL string,
) (string, error) {
	for _, ref := range refs {
		if ref.Name() != plumbing.HEAD || ref.Type() != plumbing.SymbolicReference {
			continue
		}
		if target := ref.Target(); target.IsBranch() {
			return target.Short(), nil
		}
	}
	return "", fmt.Errorf("%w: %s advertises no HEAD symref", ErrNoDefaultBranch, cloneURL)
}

// listRefs performs the one network round trip both remote questions are
// answered from.
func (c *constraintResolver) listRefs(
	ctx context.Context,
	cloneURL string,
) ([]*plumbing.Reference, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{cloneURL},
	})

	return remote.ListContext(ctx, &gogit.ListOptions{})
}

func (c *constraintResolver) resolveWithCloneURL(
	ctx context.Context,
	cloneURL string,
	pattern string,
) (string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
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

// sortTagsDesc sorts tags in descending order. Stable semver tags are compared
// numerically and always rank ahead of the rest, so a single unparseable tag
// cannot demote the whole set to string comparison — that would make v1.9.0
// outrank v1.10.0. Lexicographic order applies only when no tag is semver, and
// to the non-semver remainder.
func sortTagsDesc(tags []string) {
	semver := make([]string, 0, len(tags))
	rest := make([]string, 0, len(tags))
	for _, t := range tags {
		if IsStableSemver(t) {
			semver = append(semver, t)
			continue
		}
		rest = append(rest, t)
	}

	if len(semver) == 0 {
		sortLexDesc(tags)
		return
	}

	sort.Slice(semver, func(i, j int) bool {
		return semverGT(semver[i], semver[j])
	})
	sortLexDesc(rest)

	copy(tags, semver)
	copy(tags[len(semver):], rest)
}

func sortLexDesc(tags []string) {
	sort.Slice(tags, func(i, j int) bool {
		return tags[i] > tags[j]
	})
}

// IsStableSemver reports whether a tag names a stable release: two or three
// non-negative integer components, optionally prefixed with "v". Anything
// carrying a prerelease component — v1.2.0-rc.1, nightly — is not stable.
func IsStableSemver(tag string) bool {
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
