package picker

import (
	"regexp"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type Picker interface {
	Pick(
		repo string,
		assets []domain.ReleaseAsset,
		platform domain.OS,
	) (Pick, bool)
}

type Pick struct {
	Asset     domain.ReleaseAsset
	Format    Format
	Match     Match
	NameMatch bool
	GUI       bool
	Product   string
	Accepted  bool
}

type Format string

const (
	FormatArchive  Format = "archive"
	FormatBinary   Format = "binary"
	FormatAppImage Format = "appimage"
	FormatDMG      Format = "dmg"
	FormatMSI      Format = "msi"
)

type Match string

const (
	MatchExact    Match = "exact"
	MatchAssumed  Match = "assumed"
	MatchEmulated Match = "emulated"
)

type picker struct {
	skipExtension   *regexp.Regexp
	skipToken       *regexp.Regexp
	archive         *regexp.Regexp
	installer       *regexp.Regexp
	installerExe    *regexp.Regexp
	guiPackage      *regexp.Regexp
	stemSuffix      *regexp.Regexp
	impliedFamilies []pattern
	families        []pattern
	arches          []pattern
	known           *regexp.Regexp
	exoticArch      *regexp.Regexp
	channel         *regexp.Regexp
	companion       *regexp.Regexp
	separator       *regexp.Regexp
	nonAlnum        *regexp.Regexp
}

func New() Picker {
	return &picker{
		skipExtension: regexp.MustCompile(skipExtensionSource),
		skipToken:     bounded(ignoredWordsSource),
		archive:       regexp.MustCompile(archiveSource),
		installer:     regexp.MustCompile(installerSource),
		installerExe:  regexp.MustCompile(installerExeSource),
		guiPackage:    regexp.MustCompile(guiPackageSource),
		stemSuffix:    regexp.MustCompile(stemSource),
		impliedFamilies: []pattern{
			{label: familyDarwin, re: suffixed(dmgSuffix)},
			{label: familyWindows, re: suffixed(exeSuffix)},
			{label: familyWindows, re: suffixed(msiSuffix)},
			{label: familyLinux, re: suffixed(appImageSuffix)},
		},
		families: []pattern{
			{label: familyLinux, re: bounded(linuxSource)},
			{label: familyDarwin, re: bounded(darwinSource)},
			{label: familyWindows, re: bounded(windowsSource)},
		},
		arches: []pattern{
			{label: archARM64, re: bounded(arm64Source)},
			{label: archAMD64, re: bounded(amd64Source)},
			{label: archUniversal, re: bounded(universalSource)},
			{label: archOther, re: bounded(otherArchSource)},
		},
		known:      regexp.MustCompile(knownSource),
		exoticArch: regexp.MustCompile("^(?:" + otherArchSource + ")$"),
		channel:    regexp.MustCompile(channelSource),
		companion:  regexp.MustCompile(companionSource),
		separator:  regexp.MustCompile(separatorSource),
		nonAlnum:   regexp.MustCompile(nonAlnumSource),
	}
}

func (p *picker) Pick(
	repo string,
	assets []domain.ReleaseAsset,
	platform domain.OS,
) (Pick, bool) {
	t := targetOf(platform)
	id := p.identify(repo)
	release := p.release(assets)
	portable := keep(release, classification.notMSI)
	candidates, match := candidatesFor(portable, portable, id, t)
	if len(candidates) == 0 {
		candidates, match = candidatesFor(release, keep(release, classification.msi), id, t)
	}
	if len(candidates) == 0 {
		return Pick{}, false
	}

	gui := p.shipsGUI(assets)
	ranked := canonical(rank(candidates, id))
	if distinctStems(ranked) > 1 {
		ranked = preferPackaging(ranked, gui, t)
	}
	if distinctStems(ranked) != 1 {
		return Pick{}, false
	}

	chosen := ranked[0]
	if chosen.arch == archNone && match == MatchExact {
		match = MatchAssumed
	}
	return Pick{
		Asset:     chosen.asset,
		Format:    chosen.format,
		Match:     match,
		NameMatch: id.owns(chosen) && id.accepts(chosen),
		GUI:       gui,
		Product:   chosen.product,
		Accepted:  id.accepts(chosen),
	}, true
}

func candidatesFor(
	release []classification,
	pool []classification,
	id repoIdentity,
	t target,
) ([]classification, Match) {
	return tier(eligible(keep(pool, t.contains), release, id), t)
}

type pattern struct {
	label string
	re    *regexp.Regexp
}

func bounded(
	source string,
) *regexp.Regexp {
	return regexp.MustCompile(boundaryStart + "(?:" + source + ")" + boundaryEnd)
}

func suffixed(
	suffix string,
) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(suffix) + "$")
}

func firstMatch(
	patterns []pattern,
	name string,
) (string, bool) {
	for _, p := range patterns {
		if p.re.MatchString(name) {
			return p.label, true
		}
	}
	return "", false
}

func allMatches(
	patterns []pattern,
	name string,
) []string {
	var labels []string
	for _, p := range patterns {
		if p.re.MatchString(name) {
			labels = append(labels, p.label)
		}
	}
	return labels
}
