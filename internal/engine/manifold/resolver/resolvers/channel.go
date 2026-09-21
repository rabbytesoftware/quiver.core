package resolvers

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// StableChannel is the channel a tag belongs to when it carries no channel
// suffix at all — the structural default, not an invented name.
const StableChannel = "stable"

// tagPattern locates a 2- or 3-part numeric-dot version core anchored so
// that only a trailing channel suffix, if any, follows it to the end of the
// tag. Everything before the core is an ignorable prefix (a "v", a project
// name, a path segment) — unlike IsStableSemver, the whole tag is never
// required to be a version.
var tagPattern = regexp.MustCompile(`^(.*?)(\d+(?:\.\d+){1,2})([-_.]?[A-Za-z][A-Za-z0-9._-]*)?$`)

// ParseTag splits a tag into its version core (e.g. "1.2.0") and channel
// suffix (e.g. "rc1", "" when there is none). ok is false when the tag has
// no numeric-dot run at all — such a tag has nothing to split, and is a
// pointer-channel candidate instead (see ChannelForTag).
func ParseTag(
	tag string,
) (core, suffix string, ok bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return "", "", false
	}
	return m[2], strings.TrimLeft(m[3], "-_."), true
}

// channelSuffixPattern splits a channel suffix into its leading letters
// (the channel name) and its trailing digits (the ordinal), tolerating one
// separator between them.
var channelSuffixPattern = regexp.MustCompile(`^([A-Za-z]+)(?:[-_.]?(\d+))?$`)

// ClassifyChannel splits a channel suffix into its name and ordinal. A
// suffix with no trailing digits has no ordinal (hasOrdinal is false),
// which is what keeps a channel like "insiders" or "dev" orderable only by
// version core. A suffix that doesn't fit the letters[-_.]?digits shape at
// all (a rare, contrived tag) is returned whole as the name, with no
// ordinal — it still gets a stable, if coarse, channel identity.
func ClassifyChannel(
	suffix string,
) (name string, ordinal int, hasOrdinal bool) {
	m := channelSuffixPattern.FindStringSubmatch(suffix)
	if m == nil {
		return suffix, 0, false
	}
	if m[2] == "" {
		return strings.ToLower(m[1]), 0, false
	}
	n, _ := strconv.Atoi(m[2])
	return strings.ToLower(m[1]), n, true
}

// ChannelForTag reports which channel a tag belongs to. ok is false for a
// tag with no numeric-dot run at all — a pointer-channel candidate, whose
// identity is its own literal ref name rather than anything derived here.
func ChannelForTag(
	tag string,
) (channel string, ok bool) {
	_, suffix, ok := ParseTag(tag)
	if !ok {
		return "", false
	}
	if suffix == "" {
		return StableChannel, true
	}
	name, _, _ := ClassifyChannel(suffix)
	return name, true
}

// LatestInChannel returns the tag with the highest precedence among tags
// belonging to the given channel: highest version core wins, ties within
// the same core broken by ordinal. ok is false when no tag in tags belongs
// to channel.
func LatestInChannel(
	tags []string,
	channel string,
) (tag string, ok bool) {
	var candidates []string
	for _, t := range tags {
		c, valid := ChannelForTag(t)
		if valid && c == channel {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}

	sort.Slice(candidates, func(i, j int) bool {
		return higherPrecedence(candidates[i], candidates[j])
	})
	return candidates[0], true
}

// higherPrecedence reports whether tag a outranks tag b within the same
// channel: by version core first, then by ordinal.
func higherPrecedence(
	a, b string,
) bool {
	aCore, aSuffix, _ := ParseTag(a)
	bCore, bSuffix, _ := ParseTag(b)
	if aCore != bCore {
		return semverGT(aCore, bCore)
	}
	_, aOrd, _ := ClassifyChannel(aSuffix)
	_, bOrd, _ := ClassifyChannel(bSuffix)
	return aOrd > bOrd
}
