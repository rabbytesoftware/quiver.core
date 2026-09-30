package manifold

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// Target resolves what a selector of the given kind points at in snap: a
// channel's latest member, a constraint's highest matching tag, a pin's or a
// single-ref channel's own ref (short name, escape stripped), or a commit
// itself. A refined kind reads only the sort of ref it was classified on.
func Target(
	kind domain.SelectorKind,
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	switch kind {
	case domain.SelectorChannel:
		return channelTarget(selector, snap)
	case domain.SelectorOrderedChannel:
		return orderedTarget(selector, snap)
	case domain.SelectorConstraint:
		return constraintTarget(selector, snap)
	case domain.SelectorPin, domain.SelectorTagPin, domain.SelectorBranchPin,
		domain.SelectorPointerChannel, domain.SelectorBranchChannel:
		ref, commit, ok := pinnedRef(kind, selector, snap)
		if !ok {
			return domain.Available{}, fmt.Errorf("target %s %q: %w", kind, selector, ErrUnknownSelector)
		}
		return domain.Available{Ref: ref, Commit: commit}, nil
	case domain.SelectorCommit:
		return domain.Available{Ref: selector, Commit: selector}, nil
	}
	return domain.Available{}, fmt.Errorf("target %q: kind %q: %w", selector, kind, ErrUnknownSelector)
}

// Drift reports whether a row following selector is behind snap, and the
// target it should move to. A commit selector never drifts; an empty
// resolved state always does.
func Drift(
	kind domain.SelectorKind,
	selector string,
	resolved domain.Resolved,
	snap domain.RefSnapshot,
) (domain.Available, bool, error) {
	if kind == domain.SelectorCommit {
		return domain.Available{}, false, nil
	}

	target, err := Target(kind, selector, snap)
	if err != nil && kind == domain.SelectorChannel {
		target, err = defaultBranchTarget(selector, resolved, snap, err)
	}
	if err != nil {
		return domain.Available{}, false, fmt.Errorf("drift: %w", err)
	}
	if target.Ref == resolved.Ref && target.Commit == resolved.Commit {
		return domain.Available{}, false, nil
	}
	return target, true, nil
}

func channelTarget(
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	channel, ok := findChannel(selector, snap)
	if !ok {
		return domain.Available{}, fmt.Errorf("target channel %q: %w", selector, ErrUnknownSelector)
	}
	commit, ok := snap.Commit(channel.Latest)
	if !ok {
		return domain.Available{}, fmt.Errorf("target channel %q: latest %q has no commit: %w", selector, channel.Latest, ErrUnknownSelector)
	}
	return domain.Available{Ref: channel.Latest, Commit: commit}, nil
}

// orderedTarget reads only an ordered channel, so a rolling tag of the same
// name pushed later never answers for it.
func orderedTarget(
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	for _, c := range ChannelsOf(snap) {
		if c.Name == selector && c.Kind == "ordered" {
			return domain.Available{Ref: c.Latest, Commit: snap.Tags[c.Latest]}, nil
		}
	}
	return domain.Available{}, fmt.Errorf("target ordered channel %q: %w", selector, ErrUnknownSelector)
}

func constraintTarget(
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	ref, ok, err := resolvers.HighestMatch(sortedTags(snap), selector)
	if err != nil {
		return domain.Available{}, fmt.Errorf("target constraint %q: %w: %w", selector, ErrUnknownSelector, err)
	}
	if !ok {
		return domain.Available{}, fmt.Errorf("target constraint %q: no tag matches: %w", selector, ErrUnknownSelector)
	}
	return domain.Available{Ref: ref, Commit: snap.Tags[ref]}, nil
}

// defaultBranchTarget keeps a row that settled on a repository's default
// branch following it once the repository publishes a first release: the
// branch is listed as a channel only while there are no tags, and a later tag
// push never changes what an identity means. Only the HEAD branch the row
// itself resolved to qualifies, so an ordered channel (resolved to one of its
// tags) or a deleted pointer tag never quietly turns into a same-name branch.
func defaultBranchTarget(
	selector string,
	resolved domain.Resolved,
	snap domain.RefSnapshot,
	notFound error,
) (domain.Available, error) {
	if selector != snap.Head || resolved.Ref != selector {
		return domain.Available{}, notFound
	}
	commit, ok := snap.Branches[selector]
	if !ok {
		return domain.Available{}, notFound
	}
	return domain.Available{Ref: selector, Commit: commit}, nil
}
