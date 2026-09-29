package resolvers

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
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

// ConstraintResolver answers the questions a remote's ref advertisement can
// answer: which tag satisfies a constraint, and which branch is the default.
type ConstraintResolver interface {
	Resolve(ctx context.Context, ns domain.Namespace, pattern string) (string, error)

	// ListTags returns every tag a namespace's repository publishes,
	// unfiltered — the raw material a channel classifier buckets.
	ListTags(ctx context.Context, ns domain.Namespace) ([]string, error)

	// DefaultBranch reports the branch the remote's HEAD points at, and the
	// commit hash that branch currently resolves to. It is the repository's
	// real default branch on any git host, whatever it is named.
	DefaultBranch(ctx context.Context, ns domain.Namespace) (branch, hash string, err error)

	// RefCommit reports the commit hash the tag or branch named ref currently
	// resolves to. It is how a rolling tag that is force-moved onto a new
	// commit is told apart from the same tag left where it was.
	RefCommit(ctx context.Context, ns domain.Namespace, ref string) (string, error)

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

func (c *constraintResolver) Resolve(
	ctx context.Context,
	ns domain.Namespace,
	pattern string,
) (string, error) {
	return c.resolveWithCloneURL(ctx, ns.BareNamespace().CloneURL(), pattern)
}

func (c *constraintResolver) ListTags(
	ctx context.Context,
	ns domain.Namespace,
) ([]string, error) {
	return c.listTagsWithCloneURL(ctx, ns.BareNamespace().CloneURL())
}

func (c *constraintResolver) DefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (string, string, error) {
	return c.defaultBranchWithCloneURL(ctx, ns.BareNamespace().CloneURL())
}

func (c *constraintResolver) RefCommit(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
) (string, error) {
	return c.refCommitWithCloneURL(ctx, ns.BareNamespace().CloneURL(), ref)
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

func (c *constraintResolver) refCommitWithCloneURL(
	ctx context.Context,
	cloneURL string,
	ref string,
) (string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
	if err != nil {
		return "", fmt.Errorf("ref commit: list refs for %s: %w", cloneURL, err)
	}
	return commitOfRef(refs, ref, cloneURL)
}

// commitOfRef finds ref among a ref advertisement's tags and branches. An
// annotated tag is advertised twice, once as the tag object and once peeled to
// the commit it points at; the commit is what identifies the release, so the
// peeled entry wins.
func commitOfRef(
	refs []*plumbing.Reference,
	ref string,
	cloneURL string,
) (string, error) {
	var found string
	for _, r := range refs {
		name := r.Name()
		if !name.IsTag() && !name.IsBranch() {
			continue
		}
		short := name.Short()
		if short == ref+"^{}" {
			return r.Hash().String(), nil
		}
		if short == ref {
			found = r.Hash().String()
		}
	}
	if found == "" {
		return "", fmt.Errorf("%w: %s has no ref %q", ErrRefNotFound, cloneURL, ref)
	}
	return found, nil
}

func (c *constraintResolver) defaultBranchWithCloneURL(
	ctx context.Context,
	cloneURL string,
) (string, string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
	if err != nil {
		return "", "", fmt.Errorf("default branch: list refs for %s: %w", cloneURL, err)
	}
	return headBranch(refs, cloneURL)
}

// headBranch reads the branch a remote's HEAD points at, and that branch's own
// current commit hash. The ref advertisement carries HEAD as a symbolic
// reference, so its target names the default branch without any host-specific
// API and without guessing from a list; the branch's hash comes from that same
// ref's own entry in the same advertisement, so answering both costs no extra
// round trip.
func headBranch(
	refs []*plumbing.Reference,
	cloneURL string,
) (string, string, error) {
	target := headTarget(refs)
	if target == "" || !target.IsBranch() {
		return "", "", fmt.Errorf("%w: %s advertises no HEAD symref", ErrNoDefaultBranch, cloneURL)
	}

	for _, ref := range refs {
		if ref.Name() == target {
			return target.Short(), ref.Hash().String(), nil
		}
	}
	return "", "", fmt.Errorf("%w: %s advertises HEAD -> %s but not the ref itself", ErrNoDefaultBranch, cloneURL, target)
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

// listRefs performs the one network round trip both remote questions are
// answered from.
func (c *constraintResolver) listRefs(
	ctx context.Context,
	cloneURL string,
) ([]*plumbing.Reference, error) {
	return c.listRefsPeeling(ctx, cloneURL, gogit.IgnorePeeled)
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

func (c *constraintResolver) resolveWithCloneURL(
	ctx context.Context,
	cloneURL string,
	pattern string,
) (string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
	if err != nil {
		return "", fmt.Errorf("constraint: list refs for %s: %w", cloneURL, err)
	}

	tag, ok, err := HighestMatch(tagNames(refs), pattern)
	if err != nil {
		return "", fmt.Errorf("constraint: invalid pattern %q: %w", pattern, err)
	}
	if !ok {
		return "", fmt.Errorf("constraint: no git tags match pattern %q for %s", pattern, cloneURL)
	}
	return tag, nil
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

// tagNames extracts every tag's short name from a ref advertisement.
func tagNames(
	refs []*plumbing.Reference,
) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Name().IsTag() {
			names = append(names, ref.Name().Short())
		}
	}
	return names
}

func (c *constraintResolver) listTagsWithCloneURL(
	ctx context.Context,
	cloneURL string,
) ([]string, error) {
	refs, err := c.listRefs(ctx, cloneURL)
	if err != nil {
		return nil, fmt.Errorf("constraint: list refs for %s: %w", cloneURL, err)
	}
	return tagNames(refs), nil
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
