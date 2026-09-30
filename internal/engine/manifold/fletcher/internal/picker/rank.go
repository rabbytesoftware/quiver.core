package picker

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func tier(
	cs []classification,
	t target,
) ([]classification, Match) {
	if native := keep(cs, t.native); len(native) > 0 {
		return withGUIDMG(native, cs, t), MatchExact
	}
	if universal := keep(cs, t.universal); len(universal) > 0 {
		return withGUIDMG(universal, cs, t), MatchExact
	}
	if !t.assumesArch() {
		return nil, ""
	}
	if assumed := keep(cs, classification.archless); len(assumed) > 0 {
		return assumed, MatchAssumed
	}
	if !t.emulates() {
		return nil, ""
	}
	return keep(cs, classification.amd64), MatchEmulated
}

func withGUIDMG(
	tiered []classification,
	all []classification,
	t target,
) []classification {
	if t.family != familyDarwin {
		return tiered
	}
	dmgs := keep(all, func(c classification) bool {
		return c.format == FormatDMG && c.arch == archNone && !hasPortableTwin(c, all)
	})
	return append(slices.Clone(tiered), dmgs...)
}

func eligible(
	family []classification,
	release []classification,
	id repoIdentity,
) []classification {
	if slices.ContainsFunc(release, id.accepts) {
		return keep(family, id.accepts)
	}
	if distinctProducts(keep(release, classification.supported)) > 1 {
		return nil
	}
	return family
}

func rank(
	cs []classification,
	id repoIdentity,
) []classification {
	owned := prefer(cs, id.owns)
	scored := highestScored(owned)
	mentioned := prefer(scored, id.mentionedIn)
	return fewestExtras(mentioned, id)
}

func keep(
	cs []classification,
	predicate func(classification) bool,
) []classification {
	var out []classification
	for _, c := range cs {
		if predicate(c) {
			out = append(out, c)
		}
	}
	return out
}

func prefer(
	cs []classification,
	predicate func(classification) bool,
) []classification {
	if preferred := keep(cs, predicate); len(preferred) > 0 {
		return preferred
	}
	return cs
}

func highestScored(
	cs []classification,
) []classification {
	best := math.MinInt
	for _, c := range cs {
		best = max(best, score(c, cs))
	}
	return keep(cs, func(c classification) bool { return score(c, cs) == best })
}

func fewestExtras(
	cs []classification,
	id repoIdentity,
) []classification {
	related := keep(cs, id.extends)
	if len(related) == 0 {
		return cs
	}
	fewest := math.MaxInt
	for _, c := range related {
		fewest = min(fewest, id.extras(c))
	}
	return keep(related, func(c classification) bool { return id.extras(c) == fewest })
}

func preferPackaging(
	cs []classification,
	gui bool,
	t target,
) []classification {
	if distinctProducts(cs) != 1 {
		return cs
	}
	best := math.MaxInt
	for _, c := range cs {
		if c.format == FormatArchive || c.format == FormatAppImage {
			best = min(best, packagingRank(c, gui, t))
		}
	}
	return keep(cs, func(c classification) bool {
		if c.format != FormatArchive && c.format != FormatAppImage {
			return true
		}
		return packagingRank(c, gui, t) == best
	})
}

func packagingRank(
	c classification,
	gui bool,
	t target,
) int {
	appImageFirst := gui && t.family == familyLinux
	if (c.format == FormatAppImage) == appImageFirst {
		return 0
	}
	return 1
}

func distinctStems(
	cs []classification,
) int {
	stems := make(map[string]struct{}, len(cs))
	for _, c := range cs {
		stems[c.stem] = struct{}{}
	}
	return len(stems)
}

func canonical(
	cs []classification,
) []classification {
	sorted := slices.SortedStableFunc(slices.Values(cs), preferred)
	seen := make(map[string]struct{}, len(sorted))
	var out []classification
	for _, c := range sorted {
		if _, dup := seen[c.asset.Digest]; dup {
			continue
		}
		seen[c.asset.Digest] = struct{}{}
		out = append(out, c)
	}
	return out
}

func preferred(
	a classification,
	b classification,
) int {
	return cmp.Or(
		cmp.Compare(tieRank(a), tieRank(b)),
		strings.Compare(a.asset.Name, b.asset.Name),
	)
}

func distinctProducts(
	cs []classification,
) int {
	products := make(map[string]struct{}, len(cs))
	for _, c := range cs {
		products[c.product] = struct{}{}
	}
	return len(products)
}

const (
	portableScore = 30
	dmgScore      = 20
	guiDMGScore   = 40
	muslBonus     = 2
)

const (
	archiveRank = iota
	appImageRank
	binaryRank
	exeRank
	dmgRank
	msiRank
)

func score(
	c classification,
	siblings []classification,
) int {
	base := formatScore(c, siblings)
	if c.musl {
		return base + muslBonus
	}
	return base
}

func formatScore(
	c classification,
	siblings []classification,
) int {
	if c.format != FormatDMG {
		return portableScore
	}
	if hasPortableTwin(c, siblings) {
		return dmgScore
	}
	return guiDMGScore
}

func hasPortableTwin(
	c classification,
	siblings []classification,
) bool {
	return slices.ContainsFunc(siblings, func(other classification) bool {
		return other.format != FormatDMG && other.stem == c.stem
	})
}

func tieRank(
	c classification,
) int {
	if c.format == FormatArchive {
		return archiveRank
	}
	if c.format == FormatAppImage {
		return appImageRank
	}
	if c.format == FormatBinary {
		return binaryRankOf(c)
	}
	if c.format == FormatMSI {
		return msiRank
	}
	return dmgRank
}

func binaryRankOf(
	c classification,
) int {
	if c.exe {
		return exeRank
	}
	return binaryRank
}

type target struct {
	family string
	arch   string
}

func targetOf(
	platform domain.OS,
) target {
	switch platform {
	case domain.OSLinuxAMD64:
		return target{family: familyLinux, arch: archAMD64}
	case domain.OSLinuxARM64:
		return target{family: familyLinux, arch: archARM64}
	case domain.OSDarwinAMD64:
		return target{family: familyDarwin, arch: archAMD64}
	case domain.OSDarwinARM64:
		return target{family: familyDarwin, arch: archARM64}
	case domain.OSWindowsAMD64:
		return target{family: familyWindows, arch: archAMD64}
	case domain.OSWindowsARM64:
		return target{family: familyWindows, arch: archARM64}
	}
	return target{}
}

func (t target) contains(
	c classification,
) bool {
	return c.family == t.family
}

func (t target) native(
	c classification,
) bool {
	return c.arch == t.arch
}

func (t target) universal(
	c classification,
) bool {
	return c.arch == archUniversal && t.family == familyDarwin
}

func (t target) assumesArch() bool {
	return t.family != familyLinux || t.arch != archARM64
}

func (t target) emulates() bool {
	return t.arch == archARM64 && t.family != familyLinux
}
