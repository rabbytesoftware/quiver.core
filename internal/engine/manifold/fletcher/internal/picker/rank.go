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
		return native, MatchExact
	}
	if universal := keep(cs, t.universal); len(universal) > 0 {
		return universal, MatchExact
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
		best = max(best, score(c))
	}
	return keep(cs, func(c classification) bool { return score(c) == best })
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
	muslBonus     = 2
)

const (
	archiveRank = iota
	appImageRank
	binaryRank
	exeRank
	dmgRank
	unknownRank
)

func score(
	c classification,
) int {
	base := formatScore(c.format)
	if c.musl {
		return base + muslBonus
	}
	return base
}

func formatScore(
	format Format,
) int {
	switch format {
	case FormatArchive, FormatBinary, FormatAppImage:
		return portableScore
	case FormatDMG:
		return dmgScore
	}
	return 0
}

func tieRank(
	c classification,
) int {
	switch c.format {
	case FormatArchive:
		return archiveRank
	case FormatAppImage:
		return appImageRank
	case FormatBinary:
		return binaryRankOf(c)
	case FormatDMG:
		return dmgRank
	}
	return unknownRank
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
