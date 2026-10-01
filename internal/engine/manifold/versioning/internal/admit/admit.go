package admit

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	sel "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/selector"
)

// ErrNotAdmitted reports a ref its selector could never resolve to.
var ErrNotAdmitted = errors.New("manifold: ref not admitted by selector")

// Admit resolves ref as state declared for a row following selector, and
// refuses any state that would contradict the row's identity: a channel
// admits its members (a pointer channel, or the default-branch fallback,
// only its own ref), a constraint the tags its glob matches, a pin its own
// ref, and a commit selector itself or any ref at a commit it prefixes, hex
// compared without case. The commit selector itself is recorded as the
// selector, as an install records it. A ref snap does not hold is
// ErrUnknownSelector; one the selector could never resolve to is
// ErrNotAdmitted.
func Admit(
	kind domain.SelectorKind,
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	if kind == domain.SelectorCommit && strings.EqualFold(ref, selector) {
		return domain.Available{Ref: selector, Commit: selector}, nil
	}
	if _, ok := snap.Commit(ref); !ok {
		return domain.Available{}, fmt.Errorf("admit %q: %w", ref, sel.ErrUnknownSelector)
	}
	commit, ok := sel.RefCommit(kind, selector, ref, snap)
	if !ok || !admits(kind, selector, ref, commit, snap) {
		return domain.Available{}, fmt.Errorf("admit %q under %q: %w", ref, selector, ErrNotAdmitted)
	}
	return domain.Available{Ref: ref, Commit: commit}, nil
}

func admits(
	kind domain.SelectorKind,
	selector string,
	ref string,
	commit string,
	snap domain.RefSnapshot,
) bool {
	switch kind {
	case domain.SelectorChannel:
		return channelAdmits(selector, ref, snap)
	case domain.SelectorOrderedChannel:
		channel, ok := sel.FindChannel(selector, snap)
		return ok && channel.Kind == "ordered" && slices.Contains(channel.Members, ref)
	case domain.SelectorConstraint:
		matched, err := path.Match(selector, ref)
		return err == nil && matched
	case domain.SelectorCommit:
		return strings.HasPrefix(strings.ToLower(commit), strings.ToLower(selector))
	case domain.SelectorPin, domain.SelectorTagPin, domain.SelectorBranchPin,
		domain.SelectorPointerChannel, domain.SelectorBranchChannel:
		name, _ := sel.FollowedRef(kind, selector)
		return name == ref
	}
	return false
}

// channelAdmits serves a row stored before kinds were refined: a channel of
// its name, else the HEAD branch it settled on, which Drift keeps following
// after the first tag is published.
func channelAdmits(
	selector string,
	ref string,
	snap domain.RefSnapshot,
) bool {
	channel, ok := sel.FindChannel(selector, snap)
	if !ok {
		return selector == snap.Head && ref == selector
	}
	if channel.Kind == "ordered" {
		return slices.Contains(channel.Members, ref)
	}
	return channel.Latest == ref
}
