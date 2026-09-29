package manifold

import (
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// Target resolves what a selector of the given kind points at in snap: a
// channel's latest member, a constraint's highest matching tag, a pin's own
// ref (short name, escape stripped), or a commit itself.
func Target(
	kind domain.SelectorKind,
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	switch kind {
	case domain.SelectorChannel:
		return channelTarget(selector, snap)
	case domain.SelectorConstraint:
		return constraintTarget(selector, snap)
	case domain.SelectorPin:
		ref, commit, ok := pinnedRef(selector, snap)
		if !ok {
			return domain.Available{}, fmt.Errorf("target pin %q: %w", selector, ErrUnknownSelector)
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
