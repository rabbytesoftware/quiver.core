package manifold

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ErrNotAdmitted reports a ref its selector could never resolve to.
var ErrNotAdmitted = errors.New("manifold: ref not admitted by selector")

// Admit resolves ref as state declared for a row following selector, and
// refuses any state that would contradict the row's identity: a channel
// admits its members (a pointer channel only its own ref), a constraint the
// tags its glob matches, a pin its own ref, and a commit selector itself or
// any ref at a commit it prefixes. A ref snap does not hold is
// ErrUnknownSelector; one the selector could never resolve to is
// ErrNotAdmitted.
func Admit(
	kind domain.SelectorKind,
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	if kind == domain.SelectorCommit && ref == selector {
		return domain.Available{Ref: ref, Commit: ref}, nil
	}
	commit, ok := snap.Commit(ref)
	if !ok {
		return domain.Available{}, fmt.Errorf("admit %q: %w", ref, ErrUnknownSelector)
	}

	admitted := false
	switch kind {
	case domain.SelectorChannel:
		admitted = channelAdmits(selector, ref, snap)
	case domain.SelectorConstraint:
		admitted = constraintAdmits(selector, ref, snap)
	case domain.SelectorCommit:
		admitted = strings.HasPrefix(strings.ToLower(commit), strings.ToLower(selector))
	case domain.SelectorPin:
		return admitPin(selector, ref, snap)
	}
	if !admitted {
		return domain.Available{}, fmt.Errorf("admit %q under %q: %w", ref, selector, ErrNotAdmitted)
	}
	return domain.Available{Ref: ref, Commit: commit}, nil
}

func channelAdmits(
	selector string,
	ref string,
	snap domain.RefSnapshot,
) bool {
	channel, ok := findChannel(selector, snap)
	if !ok {
		return false
	}
	if channel.Kind == "ordered" {
		return slices.Contains(channel.Members, ref)
	}
	return channel.Latest == ref
}

// constraintAdmits takes tags only, as the constraint's own target does.
func constraintAdmits(
	selector string,
	ref string,
	snap domain.RefSnapshot,
) bool {
	if _, isTag := snap.Tags[ref]; !isTag {
		return false
	}
	matched, err := path.Match(selector, ref)
	return err == nil && matched
}

// admitPin takes the commit from the pin itself, so an escaped branch pin
// never lands on a same-name tag's commit.
func admitPin(
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	pinned, commit, ok := pinnedRef(selector, snap)
	if !ok || pinned != ref {
		return domain.Available{}, fmt.Errorf("admit %q under pin %q: %w", ref, selector, ErrNotAdmitted)
	}
	return domain.Available{Ref: ref, Commit: commit}, nil
}
