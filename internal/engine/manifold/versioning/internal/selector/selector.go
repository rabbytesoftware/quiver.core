package selector

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// ErrUnknownSelector reports a selector that names no channel, ref, glob or
// commit on the repository, or a target that is absent from its snapshot.
var ErrUnknownSelector = errors.New("manifold: unknown selector")

const (
	tagRefPrefix    = "refs/tags/"
	branchRefPrefix = "refs/heads/"
	minCommitLen    = 7
	maxCommitLen    = 40
)

// ClassifySelector decides what a selector follows: a listed channel name is
// a channel; else an exact tag or branch (or a refs/tags/, refs/heads/ escape)
// is a pin; else a glob is a constraint; else 7–40 hex characters are a commit.
// The kind is refined by the ref the selector names right now (an ordered
// channel, a rolling tag or the HEAD branch; a tag or a branch), so the row
// that stores it keeps following that ref whatever is pushed later.
// A selector with an empty ref component names nothing, whatever the snapshot
// holds: two spellings of one ref must never become two identities.
func ClassifySelector(
	selector string,
	snap domain.RefSnapshot,
) (domain.SelectorKind, error) {
	if HasEmptyComponent(selector) {
		return domain.SelectorPin, fmt.Errorf("classify selector %q: empty ref component: %w", selector, ErrUnknownSelector)
	}
	if channel, ok := FindChannel(selector, snap); ok {
		return channelKind(channel), nil
	}
	if kind, ok := pinKind(selector, snap); ok {
		return kind, nil
	}
	if isGlob(selector) {
		if _, err := path.Match(selector, ""); err != nil {
			return domain.SelectorPin, fmt.Errorf("classify selector %q: %w: %w", selector, ErrUnknownSelector, err)
		}
		return domain.SelectorConstraint, nil
	}
	if isCommitish(selector) {
		return domain.SelectorCommit, nil
	}
	return domain.SelectorPin, fmt.Errorf("classify selector %q: %w", selector, ErrUnknownSelector)
}

// DefaultChannel names the channel a refless install follows: stable when it
// has members, else the channel whose latest tag is newest, else the first
// pointer channel, else the HEAD branch of a repository with no tags.
func DefaultChannel(
	snap domain.RefSnapshot,
) (string, error) {
	channels := ChannelsOf(snap)
	if len(channels) == 0 {
		return "", fmt.Errorf("default channel: %w", ErrUnknownSelector)
	}
	if channels[0].Name == resolvers.StableChannel {
		return resolvers.StableChannel, nil
	}
	return resolvers.NewestFirst(channels)[0].Name, nil
}

// FindChannel returns the channel named selector, preferring an ordered
// channel over a pointer tag of the same name.
func FindChannel(
	selector string,
	snap domain.RefSnapshot,
) (models.ChannelInfo, bool) {
	var found models.ChannelInfo
	ok := false
	for _, c := range ChannelsOf(snap) {
		if c.Name != selector {
			continue
		}
		if c.Kind == "ordered" {
			return c, true
		}
		found, ok = c, true
	}
	return found, ok
}

func channelKind(
	channel models.ChannelInfo,
) domain.SelectorKind {
	if channel.Kind == "ordered" {
		return domain.SelectorOrderedChannel
	}
	if channel.IsDefaultBranchFallback {
		return domain.SelectorBranchChannel
	}
	return domain.SelectorPointerChannel
}

// pinKind names the ref a pin selector follows: an escape says it, else a
// tag wins over a branch of the same name.
func pinKind(
	selector string,
	snap domain.RefSnapshot,
) (domain.SelectorKind, bool) {
	name, source := FollowedRef(domain.SelectorPin, selector)
	if source != refEither {
		_, ok := source.lookup(name, snap)
		return source.pinKind(), ok
	}
	if _, ok := snap.Tags[name]; ok {
		return domain.SelectorTagPin, true
	}
	_, ok := snap.Branches[name]
	return domain.SelectorBranchPin, ok
}

// refSource is where a followed ref is looked up.
type refSource int

const (
	refEither refSource = iota
	refTag
	refBranch
)

// lookup returns the commit name has in the source; either prefers a tag.
func (src refSource) lookup(
	name string,
	snap domain.RefSnapshot,
) (string, bool) {
	switch src {
	case refTag:
		commit, ok := snap.Tags[name]
		return commit, ok
	case refBranch:
		commit, ok := snap.Branches[name]
		return commit, ok
	case refEither:
	}
	return snap.Commit(name)
}

func (src refSource) pinKind() domain.SelectorKind {
	if src == refBranch {
		return domain.SelectorBranchPin
	}
	return domain.SelectorTagPin
}

// FollowedRef names the one ref a pin or single-ref channel follows, escape
// stripped, and where to look it up: an escape wins, then the stored kind;
// an unrefined kind looks at a tag first, then a branch.
func FollowedRef(
	kind domain.SelectorKind,
	selector string,
) (string, refSource) {
	if name, found := strings.CutPrefix(selector, tagRefPrefix); found {
		return name, refTag
	}
	if name, found := strings.CutPrefix(selector, branchRefPrefix); found {
		return name, refBranch
	}
	switch kind {
	case domain.SelectorTagPin, domain.SelectorPointerChannel:
		return selector, refTag
	case domain.SelectorBranchPin, domain.SelectorBranchChannel:
		return selector, refBranch
	case domain.SelectorPin, domain.SelectorChannel, domain.SelectorOrderedChannel,
		domain.SelectorConstraint, domain.SelectorCommit:
	}
	return selector, refEither
}

// PinnedRef resolves a pin or single-ref channel to its short ref name and
// commit.
func PinnedRef(
	kind domain.SelectorKind,
	selector string,
	snap domain.RefSnapshot,
) (ref, commit string, ok bool) {
	name, source := FollowedRef(kind, selector)
	commit, ok = source.lookup(name, snap)
	return name, commit, ok
}

// RefCommit reports the commit ref names right now for a row of kind
// following selector, looked up where that row's refs live: a branch row
// reads branches, a tag-following row reads tags, so a same-name ref of the
// other sort never answers for it.
func RefCommit(
	kind domain.SelectorKind,
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (string, bool) {
	switch kind {
	case domain.SelectorCommit:
		if commit, ok := snap.Commit(ref); ok {
			return commit, true
		}
		return ref, true
	case domain.SelectorOrderedChannel, domain.SelectorConstraint:
		commit, ok := snap.Tags[ref]
		return commit, ok
	case domain.SelectorPin, domain.SelectorTagPin, domain.SelectorBranchPin,
		domain.SelectorPointerChannel, domain.SelectorBranchChannel, domain.SelectorChannel:
	}
	_, source := FollowedRef(kind, selector)
	return source.lookup(ref, snap)
}

// HasEmptyComponent reports whether selector has an empty ref component, which
// two spellings of one ref would otherwise share.
func HasEmptyComponent(
	selector string,
) bool {
	return strings.HasPrefix(selector, "/") ||
		strings.HasSuffix(selector, "/") ||
		strings.Contains(selector, "//")
}

func isGlob(
	selector string,
) bool {
	return strings.ContainsAny(selector, "*?[")
}

func isCommitish(
	selector string,
) bool {
	if len(selector) < minCommitLen || len(selector) > maxCommitLen {
		return false
	}
	for _, r := range strings.ToLower(selector) {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
