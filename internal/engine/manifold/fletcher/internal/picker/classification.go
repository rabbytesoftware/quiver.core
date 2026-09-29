package picker

import (
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type classification struct {
	asset      domain.ReleaseAsset
	family     string
	arch       string
	format     Format
	musl       bool
	exe        bool
	portable   bool
	stem       string
	product    string
	companions []string
}

func (c classification) named() bool {
	return c.product != ""
}

func (c classification) guiInstaller() bool {
	return c.exe && !c.portable
}

func (c classification) msi() bool {
	return c.format == FormatMSI
}

func (c classification) notMSI() bool {
	return !c.msi()
}

func (c classification) supported() bool {
	return c.arch != archOther
}

func (c classification) archless() bool {
	return c.arch == archNone
}

func (c classification) amd64() bool {
	return c.arch == archAMD64
}

func (p *picker) release(
	assets []domain.ReleaseAsset,
) []classification {
	gui := p.shipsGUI(assets)
	var out []classification
	for _, a := range assets {
		c, ok := p.classify(a)
		if ok && c.named() && (!gui || !c.guiInstaller()) {
			out = append(out, c)
		}
	}
	return out
}

func (p *picker) shipsGUI(
	assets []domain.ReleaseAsset,
) bool {
	for _, a := range assets {
		if p.guiPackage.MatchString(strings.ToLower(a.Name)) {
			return true
		}
	}
	return false
}

func (p *picker) classify(
	asset domain.ReleaseAsset,
) (classification, bool) {
	name := strings.ToLower(asset.Name)
	if asset.Digest == "" || p.skipped(name) {
		return classification{}, false
	}

	format, ok := p.formatOf(name)
	if !ok {
		return classification{}, false
	}

	family, ok := p.familyOf(name)
	if !ok {
		return classification{}, false
	}

	stem := p.stemSuffix.ReplaceAllString(name, "")
	tokens := p.productTokens(stem)
	return classification{
		asset:      asset,
		family:     family,
		arch:       p.archOf(name),
		format:     format,
		musl:       strings.Contains(name, musl),
		exe:        strings.HasSuffix(name, exeSuffix),
		portable:   strings.Contains(name, portableWord),
		stem:       stem,
		product:    p.normalize(strings.Join(tokens, "")),
		companions: p.companionsOf(tokens),
	}, true
}

func (p *picker) skipped(
	name string,
) bool {
	return p.skipExtension.MatchString(name) || p.skipToken.MatchString(name)
}

func (p *picker) formatOf(
	name string,
) (Format, bool) {
	if strings.HasSuffix(name, appImageSuffix) {
		return FormatAppImage, true
	}
	if strings.HasSuffix(name, dmgSuffix) {
		return FormatDMG, true
	}
	if strings.HasSuffix(name, msiSuffix) {
		return FormatMSI, true
	}
	if p.installer.MatchString(name) {
		return "", false
	}
	if strings.HasSuffix(name, exeSuffix) {
		return FormatBinary, !p.installerExe.MatchString(name)
	}
	if p.archive.MatchString(name) {
		return FormatArchive, true
	}
	tail := name[max(0, len(name)-bareBinaryTail):]
	return FormatBinary, !strings.Contains(tail, ".")
}

func (p *picker) familyOf(
	name string,
) (string, bool) {
	if implied, ok := firstMatch(p.impliedFamilies, name); ok {
		return implied, true
	}
	matched := allMatches(p.families, name)
	if len(matched) != 1 {
		return "", false
	}
	return matched[0], true
}

func (p *picker) archOf(
	name string,
) string {
	arch, _ := firstMatch(p.arches, name)
	return arch
}

func (p *picker) productTokens(
	stem string,
) []string {
	var tokens []string
	for _, token := range p.separator.Split(stem, -1) {
		if p.known.MatchString(token) || p.exoticArch.MatchString(token) || p.channel.MatchString(token) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func (p *picker) companionsOf(
	tokens []string,
) []string {
	var companions []string
	for _, token := range tokens {
		if p.companion.MatchString(token) {
			companions = append(companions, token)
		}
	}
	return companions
}

func (p *picker) normalize(
	s string,
) string {
	return p.nonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

type repoIdentity struct {
	name    string
	product string
	tokens  []string
}

func (p *picker) identify(
	repo string,
) repoIdentity {
	name := strings.ToLower(repo[strings.LastIndex(repo, "/")+1:])
	return repoIdentity{
		name:    name,
		product: p.normalize(name),
		tokens:  p.separator.Split(name, -1),
	}
}

func (id repoIdentity) accepts(
	c classification,
) bool {
	for _, companion := range c.companions {
		if !slices.Contains(id.tokens, companion) {
			return false
		}
	}
	return true
}

func (id repoIdentity) owns(
	c classification,
) bool {
	return id.product != "" && c.product == id.product
}

func (id repoIdentity) extends(
	c classification,
) bool {
	return id.product != "" && strings.HasPrefix(c.product, id.product)
}

func (id repoIdentity) mentionedIn(
	c classification,
) bool {
	return strings.Contains(strings.ToLower(c.asset.Name), id.name)
}

func (id repoIdentity) extras(
	c classification,
) int {
	return len(c.product) - len(id.product)
}
