package resolvers

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ConstraintResolver reads a remote's ref advertisement.
type ConstraintResolver interface {
	// Refs snapshots every tag, branch and the HEAD branch in one ref
	// advertisement, with annotated tags peeled to the commit they point at.
	Refs(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error)
}

const peeledSuffix = "^{}"

type constraintResolver struct {
	timeout time.Duration
}

func NewConstraintResolver(timeout time.Duration) ConstraintResolver {
	return &constraintResolver{timeout: timeout}
}

func (c *constraintResolver) Refs(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return c.refsWithCloneURL(ctx, ns.BareNamespace().CloneURL())
}

func (c *constraintResolver) refsWithCloneURL(
	ctx context.Context,
	cloneURL string,
) (domain.RefSnapshot, error) {
	refs, err := c.listRefsPeeling(ctx, cloneURL, gogit.AppendPeeled)
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return snapshotOf(nil), nil
	}
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("refs: list refs for %s: %w", cloneURL, err)
	}
	return snapshotOf(refs), nil
}

// snapshotOf folds a ref advertisement into a RefSnapshot. An annotated tag
// is advertised as its tag object and again as "<tag>^{}" peeled to its
// commit; the commit identifies the release, so the peeled entry wins.
func snapshotOf(
	refs []*plumbing.Reference,
) domain.RefSnapshot {
	snap := domain.RefSnapshot{Tags: map[string]string{}, Branches: map[string]string{}}
	peeled := make(map[string]string)
	for _, r := range refs {
		name := r.Name()
		switch {
		case name.IsBranch():
			snap.Branches[name.Short()] = r.Hash().String()
		case name.IsTag():
			if tag, ok := strings.CutSuffix(name.Short(), peeledSuffix); ok {
				peeled[tag] = r.Hash().String()
				continue
			}
			snap.Tags[name.Short()] = r.Hash().String()
		}
	}
	for tag, commit := range peeled {
		snap.Tags[tag] = commit
	}

	target := headTarget(refs)
	if _, ok := snap.Branches[target.Short()]; ok && target.IsBranch() {
		snap.Head = target.Short()
	}
	return snap
}

func headTarget(
	refs []*plumbing.Reference,
) plumbing.ReferenceName {
	for _, ref := range refs {
		if ref.Name() == plumbing.HEAD && ref.Type() == plumbing.SymbolicReference {
			return ref.Target()
		}
	}
	return ""
}

func (c *constraintResolver) listRefsPeeling(
	ctx context.Context,
	cloneURL string,
	peeling gogit.PeelingOption,
) ([]*plumbing.Reference, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{cloneURL},
	})

	return remote.ListContext(ctx, &gogit.ListOptions{PeelingOption: peeling})
}

// HighestMatch returns the highest-ranked tag matching the glob pattern, by
// the same ranking the constraint resolver uses. ok is false when none match.
func HighestMatch(
	tags []string,
	pattern string,
) (string, bool, error) {
	var matched []string
	for _, tag := range tags {
		ok, err := path.Match(pattern, tag)
		if err != nil {
			return "", false, err
		}
		if ok {
			matched = append(matched, tag)
		}
	}
	if len(matched) == 0 {
		return "", false, nil
	}

	sortTagsDesc(matched)
	return matched[0], true, nil
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

	slices.SortFunc(semver, func(a, b string) int {
		if c := compareCores(semverParts(b), semverParts(a)); c != 0 {
			return c
		}
		return strings.Compare(b, a)
	})
	sortLexDesc(rest)

	copy(tags, semver)
	copy(tags[len(semver):], rest)
}

func sortLexDesc(tags []string) {
	slices.SortFunc(tags, func(a, b string) int {
		return strings.Compare(b, a)
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

func semverParts(tag string) [4]int {
	s := strings.TrimPrefix(tag, "v")
	parts := strings.Split(s, ".")
	var out [4]int
	for i := 0; i < len(out) && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
