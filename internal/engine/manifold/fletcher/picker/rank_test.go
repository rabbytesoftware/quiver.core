package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

func classes(names ...string) []classification {
	out := make([]classification, 0, len(names))
	for _, n := range names {
		out = append(out, classification{asset: asset(n), stem: n})
	}
	return out
}

func names(cs []classification) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.asset.Name)
	}
	return out
}

func TestRank_Tier_Order(t *testing.T) {
	amd := classification{asset: asset("amd"), arch: archAMD64}
	arm := classification{asset: asset("arm"), arch: archARM64}
	none := classification{asset: asset("none"), arch: archNone}
	uni := classification{asset: asset("uni"), arch: archUniversal}

	testCases := []struct {
		name      string
		cs        []classification
		target    target
		want      []string
		wantMatch Match
	}{
		{name: "exact first", cs: []classification{amd, arm, none}, target: target{family: familyLinux, arch: archARM64}, want: []string{"arm"}, wantMatch: MatchExact},
		{name: "universal exact on darwin", cs: []classification{uni, none}, target: target{family: familyDarwin, arch: archARM64}, want: []string{"uni"}, wantMatch: MatchExact},
		{name: "native before universal", cs: []classification{uni, arm}, target: target{family: familyDarwin, arch: archARM64}, want: []string{"arm"}, wantMatch: MatchExact},
		{name: "assumed", cs: []classification{arm, none}, target: target{family: familyLinux, arch: archAMD64}, want: []string{"none"}, wantMatch: MatchAssumed},
		{name: "never assume linux arm64", cs: []classification{amd, none}, target: target{family: familyLinux, arch: archARM64}},
		{name: "emulated", cs: []classification{amd, uni}, target: target{family: familyWindows, arch: archARM64}, want: []string{"amd"}, wantMatch: MatchEmulated},
		{name: "no emulation on amd64", cs: []classification{arm}, target: target{family: familyDarwin, arch: archAMD64}},
		{name: "emulation finds nothing", cs: []classification{uni}, target: target{family: familyWindows, arch: archARM64}},
		{name: "empty", target: target{family: familyLinux, arch: archAMD64}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, match := tier(tc.cs, tc.target)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, names(got))
			assert.Equal(t, tc.wantMatch, match)
		})
	}
}

func TestRank_Prefer_FallsBack(t *testing.T) {
	cs := classes("a", "b")
	isB := func(c classification) bool { return c.asset.Name == "b" }
	isZ := func(c classification) bool { return c.asset.Name == "z" }

	assert.Equal(t, []string{"b"}, names(prefer(cs, isB)))
	assert.Equal(t, []string{"a", "b"}, names(prefer(cs, isZ)))
}

func TestRank_HighestScored_KeepsTies(t *testing.T) {
	cs := []classification{
		{asset: asset("dmg"), format: FormatDMG},
		{asset: asset("zip"), format: FormatArchive},
		{asset: asset("bin"), format: FormatBinary},
	}

	assert.Equal(t, []string{"zip", "bin"}, names(highestScored(cs)))
}

func TestRank_FewestExtras_KeepsTies(t *testing.T) {
	id := repoIdentity{name: "tool", product: "tool"}
	cs := []classification{
		{asset: asset("server"), product: "toolserver"},
		{asset: asset("a"), product: "tool"},
		{asset: asset("b"), product: ""},
	}

	assert.Equal(t, []string{"a"}, names(fewestExtras(cs, id)))
	assert.Equal(t, []string{"server", "a", "b"}, names(fewestExtras(cs, repoIdentity{name: "zap", product: "zap"})))
}

func TestRank_DistinctStems_Counts(t *testing.T) {
	testCases := []struct {
		name  string
		stems []string
		want  int
	}{
		{name: "empty", want: 0},
		{name: "one", stems: []string{"a"}, want: 1},
		{name: "duplicates", stems: []string{"a", "a"}, want: 1},
		{name: "two", stems: []string{"a", "b", "a"}, want: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, distinctStems(classes(tc.stems...)))
		})
	}
}

func TestRank_Canonical_SortsAndDedupes(t *testing.T) {
	c := func(name string, format Format, digest string) classification {
		return classification{asset: hosts.Asset{Name: name, Digest: digest}, format: format}
	}
	cs := []classification{
		c("b.dmg", FormatDMG, "d1"),
		{asset: hosts.Asset{Name: "b.exe", Digest: "d2"}, format: FormatBinary, exe: true},
		c("b", FormatBinary, "d3"),
		c("b.AppImage", FormatAppImage, "d4"),
		c("b.zip", FormatArchive, "d5"),
		c("a.zip", FormatArchive, "d5"),
	}

	assert.Equal(t, []string{"a.zip", "b.AppImage", "b", "b.exe", "b.dmg"}, names(canonical(cs)))
	assert.Empty(t, canonical(nil))
}

func TestRank_Preferred_Order(t *testing.T) {
	testCases := []struct {
		name string
		a    classification
		b    classification
		want int
	}{
		{name: "format first", a: classification{asset: asset("z.zip"), format: FormatArchive}, b: classification{asset: asset("a.dmg"), format: FormatDMG}, want: -1},
		{name: "name second", a: classification{asset: asset("b.zip"), format: FormatArchive}, b: classification{asset: asset("a.zip"), format: FormatArchive}, want: 1},
		{name: "equal", a: classification{asset: asset("a.zip"), format: FormatArchive}, b: classification{asset: asset("a.zip"), format: FormatArchive}, want: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, preferred(tc.a, tc.b))
		})
	}
}

func TestRank_Eligible_CompanionPreference(t *testing.T) {
	id := repoIdentity{name: "foo", product: "foo", tokens: []string{"foo"}}
	plain := classification{asset: asset("foo"), product: "foo"}
	cli := classification{asset: asset("foo-cli"), product: "foocli", companions: []string{"cli"}}
	cliArm := classification{asset: asset("foo-cli-arm"), product: "foocli", companions: []string{"cli"}}
	server := classification{asset: asset("foo-server"), product: "fooserver", companions: []string{"server"}}

	testCases := []struct {
		name    string
		family  []classification
		release []classification
		want    []string
	}{
		{name: "companion dropped when plain exists", family: []classification{cli, plain}, release: []classification{cli, plain}, want: []string{"foo"}},
		{name: "companion dropped when plain exists for another os", family: []classification{cli}, release: []classification{cli, plain}},
		{name: "single companion product kept", family: []classification{cli, cliArm}, release: []classification{cli, cliArm}, want: []string{"foo-cli", "foo-cli-arm"}},
		{name: "different companion products refused", family: []classification{cli}, release: []classification{cli, server}},
		{name: "exotic arch product ignored", family: []classification{cli}, release: []classification{cli, {asset: asset("foo-cli-armv7"), product: "foocliarmv7", companions: []string{"cli"}, arch: archOther}}, want: []string{"foo-cli"}},
		{name: "empty", family: nil, release: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := eligible(tc.family, tc.release, id)

			if len(tc.want) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, names(got))
		})
	}
}

func TestRank_DistinctProducts_Counts(t *testing.T) {
	assert.Equal(t, 0, distinctProducts(nil))
	assert.Equal(t, 1, distinctProducts([]classification{{product: "a"}, {product: "a"}}))
	assert.Equal(t, 2, distinctProducts([]classification{{product: "a"}, {product: "b"}}))
}
