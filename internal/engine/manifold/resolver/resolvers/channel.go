package resolvers

import (
	"cmp"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// StableChannel is the channel a tag belongs to when it carries no channel
// suffix at all — the structural default, not an invented name.
const StableChannel = "stable"

// tagPatternWithOrdinalSuffix locates a 2- or 3-part numeric-dot version core
// anchored so that only a trailing channel suffix, if any, follows it to the
// end of the tag. Everything before the core is an ignorable prefix (a "v", a
// project name, a path segment) — unlike IsStableSemver, the whole tag is
// never required to be a version. It tolerates a trailing separator followed
// by pure digits after the core (e.g. "beta-26.5-1"), which the set-aware
// classifier (classifyAll) tells apart from noise using sibling tags.
var tagPatternWithOrdinalSuffix = regexp.MustCompile(`^(.*?)(\d+(?:\.\d+){1,2})([-_.]?[A-Za-z][A-Za-z0-9._-]*|[-_.]?\d+)?$`)

// tagPatternWithDateCore locates a YYYY-MM-DD date standing where a version
// core would, with an optional .N patch after it, as the release workflows
// name a dated series: "stable-2026-09-27", "stable-2026-09-27.1",
// "hotfix-2026-09-27.1-1" (a rebuild), "beta-2026-09-27-1".
var tagPatternWithDateCore = regexp.MustCompile(`^(.*?)(\d{4})-(\d{1,2})-(\d{1,2})(?:\.(\d+))?([-_.]?[A-Za-z][A-Za-z0-9._-]*|[-_.]?\d+)?$`)

// dateEpoch is subtracted from a date core's year so a date orders among
// calendar-versioned YY.M cores by its release month: 2026-09-27 ranks as
// 26.9.27, after 26.5.1 and before 26.10.
const dateEpoch = 2000

// parseTagFull splits a tag into its prefix, version core and channel suffix.
// A date is tried first, so its dashes never make a dotted core of its day
// and patch; its core is rewritten as YY.MM.DD.patch. A date counts only
// behind a channel word (stable-, beta-, hotfix-, ...): a bare dated
// snapshot tag in a repository that releases v1.4.0 is never ranked against
// its versions. ok is false when the tag has neither a date nor a
// numeric-dot run — a pointer-channel candidate instead.
func parseTagFull(
	tag string,
) (prefix, core, suffix string, ok bool) {
	if m := tagPatternWithDateCore.FindStringSubmatch(tag); m != nil && isKnownChannel(normalizeVersionPrefix(m[1])) && validDate(m[2], m[3], m[4]) {
		return m[1], dateCore(m[2], m[3], m[4], m[5]), strings.TrimLeft(m[6], "-_."), true
	}
	if m := tagPatternWithOrdinalSuffix.FindStringSubmatch(tag); m != nil {
		return m[1], m[2], strings.TrimLeft(m[3], "-_."), true
	}
	return "", "", "", false
}

// validDate accepts a date of this century only: anything else behind a
// channel word (stable-1999-01-01, stable-9999-99-99) is no date.
func validDate(
	year, month, day string,
) bool {
	y, _ := strconv.Atoi(year)
	m, _ := strconv.Atoi(month)
	d, _ := strconv.Atoi(day)
	return y >= dateEpoch && y < dateEpoch+100 && m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// dateCore writes a date and its patch as the core YY.MM.DD.patch.
func dateCore(
	year, month, day, patch string,
) string {
	y, _ := strconv.Atoi(year)
	y -= dateEpoch
	if patch == "" {
		patch = "0"
	}
	return strconv.Itoa(y) + "." + month + "." + day + "." + patch
}

// isDateCore reports a core parseTagFull wrote from a date: the only one
// with four components.
func isDateCore(core string) bool {
	return strings.Count(core, ".") == 3
}

// knownChannels are prefixes that name a release channel whatever else the
// repository tags: "beta-26.5" is beta even when every tag is beta-prefixed.
var knownChannels = []string{
	"alpha", "beta", "canary", "dev", "edge", "hotfix", "insiders", "next", "nightly", "preview", "rc", StableChannel,
}

func isKnownChannel(
	prefix string,
) bool {
	return slices.Contains(knownChannels, prefix)
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

// normalizeVersionPrefix strips a trailing separator from a tag's prefix
// and reports "" for a bare "v" (case-insensitive) — the one, pre-existing,
// purely mechanical version marker this codebase has always stripped
// silently, not a channel name. Every other prefix is returned as-is, lowercased,
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
	core    [4]int
	ordinal int
	dated   bool
	// tier ranks a semantic version above every dated member of its channel.
	tier int
}

// calendarMajor is the smallest major a calendar version (YY.M) carries; a
// core below it is a semantic version, which dates are never ranked against.
const calendarMajor = 20

// tierAgainstDates picks one order for a channel that holds dated tags, so
// the order stays total. When every other member is a semantic version
// (major below calendarMajor), those rank above all the dates: a third-party
// stable-2019-05-01 never outranks v2.0.0. When any member is calendar-like
// (26.10, or any major of calendarMajor or more), the whole channel ranks
// numerically on one calendar, dates as YY.MM.DD, as quiver.core's own tags
// do.
func tierAgainstDates(members []classifiedTag) {
	if !slices.ContainsFunc(members, func(c classifiedTag) bool { return c.dated }) {
		return
	}
	if slices.ContainsFunc(members, func(c classifiedTag) bool { return !c.dated && c.core[0] >= calendarMajor }) {
		return
	}
	for i := range members {
		if !members[i].dated {
			members[i].tier = 1
		}
	}
}

// TagChannel is one ordered channel of a tag set: its classified name and
// its members, highest precedence first.
type TagChannel struct {
	Name    string
	Members []string
}

// classifyAll classifies every tag in tags that has a version core at all
// (a tag with none is a pointer-channel candidate, omitted here — callers
// already handle "not found" as pointer, via parseTagFull's own ok return).
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
		if !isKnownChannel(norm) {
			prefixValues[norm] = true
		}
	}

	prefixIsMeaningful := len(prefixValues) > 1

	result := make([]classifiedTag, 0, len(entries))
	for _, e := range entries {
		channel, ordinal := classifyOne(e.suffix, e.prefix, prefixIsMeaningful)
		result = append(result, classifiedTag{tag: e.tag, channel: channel, core: semverParts(e.core), ordinal: ordinal, dated: isDateCore(e.core)})
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
	if prefix != "" && (prefixIsMeaningful || isKnownChannel(prefix)) {
		return prefix, ordinal
	}
	return StableChannel, ordinal
}

// Outranks reports whether tag a ranks above tag b within one channel
// (version or date core, then ordinal). Tags of different channels, read
// each on its own ("v1.2.0-rc.1" is rc, "v1.2.0" stable), never outrank one
// another, and neither does a tag with no core.
func Outranks(
	a, b string,
) bool {
	rankA, okA := rankOf(a)
	rankB, okB := rankOf(b)
	if !okA || !okB || rankA.channel != rankB.channel {
		return false
	}
	pair := []classifiedTag{rankA, rankB}
	tierAgainstDates(pair)
	return compareClassified(pair[0], pair[1]) < 0
}

// ConstraintOutranks reports whether tag a ranks above tag b in the order a
// constraint picks its target in (HighestMatch).
func ConstraintOutranks(
	a, b string,
) bool {
	tags := []string{b, a}
	sortTagsDesc(tags)
	return tags[0] == a && a != b
}

func rankOf(
	tag string,
) (classifiedTag, bool) {
	prefix, core, suffix, ok := parseTagFull(tag)
	if !ok {
		return classifiedTag{}, false
	}
	channel, ordinal := classifyOne(suffix, normalizeVersionPrefix(prefix), false)
	return classifiedTag{tag: tag, channel: channel, core: semverParts(core), ordinal: ordinal, dated: isDateCore(core)}, true
}

// GroupChannels classifies tags once and buckets them into ordered
// channels, alphabetically by name, each member list in precedence order.
func GroupChannels(
	tags []string,
) []TagChannel {
	byChannel := make(map[string][]classifiedTag)
	for _, c := range classifyAll(tags) {
		byChannel[c.channel] = append(byChannel[c.channel], c)
	}

	out := make([]TagChannel, 0, len(byChannel))
	for _, name := range slices.Sorted(maps.Keys(byChannel)) {
		candidates := byChannel[name]
		tierAgainstDates(candidates)
		slices.SortFunc(candidates, compareClassified)
		members := make([]string, len(candidates))
		for i, c := range candidates {
			members[i] = c.tag
		}
		out = append(out, TagChannel{Name: name, Members: members})
	}
	return out
}

// SortInChannel returns every tag belonging to channel, ordered by
// precedence (highest first). Empty when no tag in tags belongs to channel.
func SortInChannel(
	tags []string,
	channel string,
) []string {
	for _, c := range GroupChannels(tags) {
		if c.Name == channel {
			return c.Members
		}
	}
	return nil
}

// compareClassified orders two tags of one channel, highest precedence
// first: by version core, then by ordinal, then by name, so equal-rank tags
// such as v1.2 and v1.2.0 always settle the same way.
func compareClassified(
	a, b classifiedTag,
) int {
	if c := cmp.Compare(b.tier, a.tier); c != 0 {
		return c
	}
	if c := compareCores(b.core, a.core); c != 0 {
		return c
	}
	if c := cmp.Compare(b.ordinal, a.ordinal); c != 0 {
		return c
	}
	return strings.Compare(b.tag, a.tag)
}

func compareCores(
	a, b [4]int,
) int {
	for i := range a {
		if c := cmp.Compare(a[i], b[i]); c != 0 {
			return c
		}
	}
	return 0
}

// ChannelsPresent returns the distinct ordered-channel names present in
// tags, alphabetically (a tag with no version core at all is a
// pointer-channel candidate, not included here — see parseTagFull).
func ChannelsPresent(
	tags []string,
) []string {
	groups := GroupChannels(tags)
	out := make([]string, len(groups))
	for i, c := range groups {
		out[i] = c.Name
	}
	return out
}
