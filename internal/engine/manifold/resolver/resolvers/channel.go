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

// tagPatternWithOrdinalSuffix additionally tolerates a trailing separator
// followed by pure digits after the version core (e.g. "beta-26.5-1") —
// needed only by the set-aware classifier below (classifyAll), which has
// sibling tags to tell a genuine prefix-borne ordinal apart from noise.
// The single-tag ParseTag/ChannelForTag path has no such context and
// deliberately keeps requiring a letter-led suffix, unchanged from before
// this fix — this is why the two patterns are kept separate rather than
// widening tagPattern itself.
var tagPatternWithOrdinalSuffix = regexp.MustCompile(`^(.*?)(\d+(?:\.\d+){1,2})([-_.]?[A-Za-z][A-Za-z0-9._-]*|[-_.]?\d+)?$`)

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

// parseTagFull is like ParseTag but also returns the prefix, and tolerates
// a trailing pure-digit continuation after the core that ParseTag itself
// does not — used only by the set-aware classifier below.
func parseTagFull(
	tag string,
) (prefix, core, suffix string, ok bool) {
	m := tagPatternWithOrdinalSuffix.FindStringSubmatch(tag)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], strings.TrimLeft(m[3], "-_."), true
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
		return strings.ToLower(suffix), 0, false
	}
	if m[2] == "" {
		return strings.ToLower(m[1]), 0, false
	}
	n, _ := strconv.Atoi(m[2])
	return strings.ToLower(m[1]), n, true
}

// ChannelForTag reports which channel a tag belongs to, using only that
// tag's own suffix — it has no sibling tags to compare against, so it
// cannot recognize a prefix-style discriminator (e.g. "beta-1.2.0"); see
// classifyAll (used by SortInChannel/ChannelsPresent) for the set-aware
// classification that can. ok is false for a tag with no numeric-dot run
// at all — a pointer-channel candidate, whose identity is its own literal
// ref name rather than anything derived here.
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

// normalizeVersionPrefix strips a trailing separator from a tag's prefix
// and reports "" for a bare "v" (case-insensitive) — the one, pre-existing,
// purely mechanical version marker this codebase has always stripped
// silently (see the original ParseTag doc comment's own "v" example), not
// a channel name. Every other prefix is returned as-is, lowercased,
// unstripped further: whether it's a genuine channel marker (like "beta-")
// or incidental noise (a project name prefix) is not decidable from the
// prefix's shape alone — see classifyAll for how that's actually decided.
func normalizeVersionPrefix(
	prefix string,
) string {
	trimmed := strings.TrimRight(prefix, "-_.")
	if strings.EqualFold(trimmed, "v") {
		return ""
	}
	return strings.ToLower(trimmed)
}

// classifiedTag is one tag's classification within the context of the
// full set it was classified against.
type classifiedTag struct {
	tag     string
	channel string
	core    string
	ordinal int
}

// classifyAll classifies every tag in tags that has a version core at all
// (a tag with none is a pointer-channel candidate, omitted here — callers
// already handle "not found" as pointer, via ParseTag's own ok return).
//
// A channel suffix (text after the version core) always wins when present,
// exactly as before. Only when a tag has no suffix does its prefix (text
// before the core) get a chance — and only if that prefix is not the sole,
// uniform prefix shared by every tag in the set. A prefix uniform across
// the whole set (most commonly a bare "v") carries no information — it
// can't distinguish anything, so it means nothing, the same reasoning
// "no suffix at all" already uses to mean "stable" rather than nothing. A
// prefix that varies — some tags have it, others don't, or different tags
// have different ones — is exactly the kind of distinguishing signal a
// channel discriminator is supposed to be, whatever text it actually is.
//
// A tag whose suffix is purely numeric (e.g. "beta-26.5-1"'s "1") is not
// itself a channel name — a bare number names nothing. It's an ordinal
// that belongs to whatever the tag's prefix already discriminates, the
// prefix-side counterpart to how "rc1"'s trailing digit is already an
// ordinal within the "rc" suffix, not a channel of its own.
func classifyAll(
	tags []string,
) []classifiedTag {
	type entry struct {
		tag, core, suffix, prefix string
	}

	var entries []entry
	prefixValues := make(map[string]bool)
	for _, t := range tags {
		prefix, core, suffix, ok := parseTagFull(t)
		if !ok {
			continue
		}
		norm := normalizeVersionPrefix(prefix)
		entries = append(entries, entry{tag: t, core: core, suffix: suffix, prefix: norm})
		prefixValues[norm] = true
	}

	prefixIsMeaningful := len(prefixValues) > 1

	result := make([]classifiedTag, 0, len(entries))
	for _, e := range entries {
		channel, ordinal := classifyOne(e.suffix, e.prefix, prefixIsMeaningful)
		result = append(result, classifiedTag{tag: e.tag, channel: channel, core: e.core, ordinal: ordinal})
	}
	return result
}

// classifyOne is classifyAll's per-tag decision, split out for clarity.
func classifyOne(
	suffix string,
	prefix string,
	prefixIsMeaningful bool,
) (channel string, ordinal int) {
	if suffix == "" {
		return classifyByPrefix(prefix, prefixIsMeaningful, 0)
	}
	if n, err := strconv.Atoi(suffix); err == nil {
		return classifyByPrefix(prefix, prefixIsMeaningful, n)
	}
	name, ord, _ := ClassifyChannel(suffix)
	return name, ord
}

// classifyByPrefix is classifyOne's fallback when the suffix itself names no
// channel (empty, or purely numeric): the prefix gets a chance instead, but
// only when it's meaningful — otherwise the tag is stable. Either way the
// already-parsed ordinal (0 when the suffix was empty) is preserved: a bare
// numeric suffix still names a specific build, and that still breaks ties
// within whichever channel the tag lands in, prefix or no prefix.
func classifyByPrefix(
	prefix string,
	prefixIsMeaningful bool,
	ordinal int,
) (channel string, ord int) {
	if prefixIsMeaningful && prefix != "" {
		return prefix, ordinal
	}
	return StableChannel, ordinal
}

// SortInChannel returns every tag belonging to channel, ordered by
// precedence (highest first): highest version core wins, ties within the
// same core broken by ordinal. Empty when no tag in tags belongs to
// channel.
func SortInChannel(
	tags []string,
	channel string,
) []string {
	classified := classifyAll(tags)
	var candidates []classifiedTag
	for _, c := range classified {
		if c.channel == channel {
			candidates = append(candidates, c)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return higherPrecedenceClassified(candidates[i], candidates[j])
	})
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.tag
	}
	return out
}

// LatestInChannel returns the tag with the highest precedence among tags
// belonging to the given channel: highest version core wins, ties within
// the same core broken by ordinal. ok is false when no tag in tags belongs
// to channel.
func LatestInChannel(
	tags []string,
	channel string,
) (tag string, ok bool) {
	sorted := SortInChannel(tags, channel)
	if len(sorted) == 0 {
		return "", false
	}
	return sorted[0], true
}

// higherPrecedenceClassified reports whether a outranks b within the same
// channel: by version core first, then by ordinal.
func higherPrecedenceClassified(
	a, b classifiedTag,
) bool {
	if a.core != b.core {
		return semverGT(a.core, b.core)
	}
	return a.ordinal > b.ordinal
}

// ChannelsPresent returns the distinct ordered-channel names present in
// tags (a tag with no version core at all is a pointer-channel candidate,
// not included here — see ParseTag). Order is alphabetical, for
// determinism; callers needing precedence order call SortInChannel per
// channel.
func ChannelsPresent(
	tags []string,
) []string {
	seen := make(map[string]bool)
	for _, c := range classifyAll(tags) {
		seen[c.channel] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
