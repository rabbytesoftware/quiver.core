package manifold

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
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
// A selector with an empty ref component names nothing, whatever the snapshot
// holds: two spellings of one ref must never become two identities.
func ClassifySelector(
	selector string,
	snap domain.RefSnapshot,
) (domain.SelectorKind, error) {
	if hasEmptyComponent(selector) {
		return domain.SelectorPin, fmt.Errorf("classify selector %q: empty ref component: %w", selector, ErrUnknownSelector)
	}
	if _, ok := findChannel(selector, snap); ok {
		return domain.SelectorChannel, nil
	}
	if _, _, ok := pinnedRef(selector, snap); ok {
		return domain.SelectorPin, nil
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
// has members, else the first listed channel, else the HEAD branch of a
// repository with no tags.
func DefaultChannel(
	snap domain.RefSnapshot,
) (string, error) {
	channels := ChannelsOf(snap)
	if len(channels) == 0 {
		return "", fmt.Errorf("default channel: %w", ErrUnknownSelector)
	}
	return channels[0].Name, nil
}

// findChannel returns the channel named selector, preferring an ordered
// channel over a pointer tag of the same name.
func findChannel(
	selector string,
	snap domain.RefSnapshot,
) (ChannelInfo, bool) {
	var found ChannelInfo
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

// pinnedRef resolves a pin selector to its short ref name and commit.
func pinnedRef(
	selector string,
	snap domain.RefSnapshot,
) (ref, commit string, ok bool) {
	if name, found := strings.CutPrefix(selector, tagRefPrefix); found {
		commit, ok = snap.Tags[name]
		return name, commit, ok
	}
	if name, found := strings.CutPrefix(selector, branchRefPrefix); found {
		commit, ok = snap.Branches[name]
		return name, commit, ok
	}
	commit, ok = snap.Commit(selector)
	return selector, commit, ok
}

func hasEmptyComponent(
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
