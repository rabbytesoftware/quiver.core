package resolvers

import "testing"

func TestParseTag_VersionCoreAndSuffix(t *testing.T) {
	testCases := []struct {
		name       string
		tag        string
		wantCore   string
		wantSuffix string
		wantOK     bool
	}{
		{name: "bare version", tag: "1.2.0", wantCore: "1.2.0", wantSuffix: "", wantOK: true},
		{name: "v prefix", tag: "v1.2.0", wantCore: "1.2.0", wantSuffix: "", wantOK: true},
		{name: "two part version", tag: "v1.5", wantCore: "1.5", wantSuffix: "", wantOK: true},
		{name: "project name prefix", tag: "myapp_v1.2.0", wantCore: "1.2.0", wantSuffix: "", wantOK: true},
		{name: "release candidate suffix no separator", tag: "1.2.0-rc1", wantCore: "1.2.0", wantSuffix: "rc1", wantOK: true},
		{name: "dotted suffix", tag: "1.2.0.beta3", wantCore: "1.2.0", wantSuffix: "beta3", wantOK: true},
		{name: "prefix and dotted suffix together", tag: "release-1.2.0-beta.3", wantCore: "1.2.0", wantSuffix: "beta.3", wantOK: true},
		{name: "no numeric run at all", tag: "nightly", wantCore: "", wantSuffix: "", wantOK: false},
		{name: "empty string", tag: "", wantCore: "", wantSuffix: "", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			core, suffix, ok := ParseTag(tc.tag)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if core != tc.wantCore {
				t.Errorf("core = %q, want %q", core, tc.wantCore)
			}
			if suffix != tc.wantSuffix {
				t.Errorf("suffix = %q, want %q", suffix, tc.wantSuffix)
			}
		})
	}
}

func TestClassifyChannel_NameAndOrdinal(t *testing.T) {
	testCases := []struct {
		name           string
		suffix         string
		wantName       string
		wantOrdinal    int
		wantHasOrdinal bool
	}{
		{name: "letters then digits no separator", suffix: "rc1", wantName: "rc", wantOrdinal: 1, wantHasOrdinal: true},
		{name: "letters dot digits", suffix: "beta.3", wantName: "beta", wantOrdinal: 3, wantHasOrdinal: true},
		{name: "letters only, no ordinal", suffix: "insiders", wantName: "insiders", wantOrdinal: 0, wantHasOrdinal: false},
		{name: "uppercase normalizes to lowercase", suffix: "RC2", wantName: "rc", wantOrdinal: 2, wantHasOrdinal: true},
		{name: "suffix that doesn't fit the letters-then-digits shape falls back whole, normalized", suffix: "Beta-2-Experimental", wantName: "beta-2-experimental", wantOrdinal: 0, wantHasOrdinal: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name, ordinal, hasOrdinal := ClassifyChannel(tc.suffix)
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if ordinal != tc.wantOrdinal {
				t.Errorf("ordinal = %d, want %d", ordinal, tc.wantOrdinal)
			}
			if hasOrdinal != tc.wantHasOrdinal {
				t.Errorf("hasOrdinal = %v, want %v", hasOrdinal, tc.wantHasOrdinal)
			}
		})
	}
}

func TestChannelForTag_Classification(t *testing.T) {
	testCases := []struct {
		name        string
		tag         string
		wantChannel string
		wantOK      bool
	}{
		{name: "stable release has no suffix", tag: "v1.4.0", wantChannel: "stable", wantOK: true},
		{name: "release candidate", tag: "v1.5.0-rc2", wantChannel: "rc", wantOK: true},
		{name: "beta with dotted ordinal", tag: "1.2.0-beta.3", wantChannel: "beta", wantOK: true},
		{name: "pointer-shaped tag has no channel", tag: "nightly", wantChannel: "", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel, ok := ChannelForTag(tc.tag)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if channel != tc.wantChannel {
				t.Errorf("channel = %q, want %q", channel, tc.wantChannel)
			}
		})
	}
}

func TestLatestInChannel_PicksHighestPrecedence(t *testing.T) {
	testCases := []struct {
		name    string
		tags    []string
		channel string
		want    string
		wantOK  bool
	}{
		{
			name:    "rc ordinals group into one channel across releases",
			tags:    []string{"v1.2.0-rc1", "v1.2.0-rc2", "v1.3.0-rc1"},
			channel: "rc",
			want:    "v1.3.0-rc1",
			wantOK:  true,
		},
		{
			name:    "same version core, ordinal breaks the tie",
			tags:    []string{"v1.2.0-rc1", "v1.2.0-rc2"},
			channel: "rc",
			want:    "v1.2.0-rc2",
			wantOK:  true,
		},
		{
			name:    "stable channel ignores prerelease tags",
			tags:    []string{"v1.4.0", "v1.5.0-rc1", "v1.3.0"},
			channel: "stable",
			want:    "v1.4.0",
			wantOK:  true,
		},
		{
			name:    "no tag in the requested channel",
			tags:    []string{"v1.4.0"},
			channel: "beta",
			want:    "",
			wantOK:  false,
		},
		{
			name:    "empty tag list",
			tags:    nil,
			channel: "stable",
			want:    "",
			wantOK:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := LatestInChannel(tc.tags, tc.channel)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("got = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeVersionPrefix_StripsSeparatorAndBareV(t *testing.T) {
	testCases := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "bare v", prefix: "v", want: ""},
		{name: "bare V uppercase", prefix: "V", want: ""},
		{name: "beta with trailing hyphen", prefix: "beta-", want: "beta"},
		{name: "project name plus v is not bare v", prefix: "myapp_v", want: "myapp_v"},
		{name: "empty prefix", prefix: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeVersionPrefix(tc.prefix)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChannelsPresent_RealWorldPrefixStyleConvention(t *testing.T) {
	// This is quiver.core's own actual, live GitHub tag set — the exact
	// tags that exposed this bug in a live smoke test against the real
	// repo. Never simplify this fixture back to synthetic suffix-style
	// tags; the whole point of this test is guarding against a shape of
	// tag this project's own releases actually use.
	tags := []string{
		"stable-26.5.1", "stable-26.5",
		"beta-26.5", "beta-26.5-1", "beta-26.5-2", "beta-26.5-3", "beta-26.5-4",
		"nightly-latest",
	}

	got := ChannelsPresent(tags)
	want := []string{"beta", "stable"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSortInChannel_RealWorldPrefixStyleConvention_StableNeverIncludesBeta(t *testing.T) {
	tags := []string{
		"stable-26.5.1", "stable-26.5",
		"beta-26.5", "beta-26.5-1", "beta-26.5-2", "beta-26.5-3", "beta-26.5-4",
		"nightly-latest",
	}

	got := SortInChannel(tags, "stable")
	want := []string{"stable-26.5.1", "stable-26.5"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v — beta-26.5 must NOT appear in the stable channel", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSortInChannel_RealWorldPrefixStyleConvention_BetaGroupsAllFiveByOrdinal(t *testing.T) {
	tags := []string{
		"stable-26.5.1", "stable-26.5",
		"beta-26.5", "beta-26.5-1", "beta-26.5-2", "beta-26.5-3", "beta-26.5-4",
		"nightly-latest",
	}

	got := SortInChannel(tags, "beta")
	want := []string{"beta-26.5-4", "beta-26.5-3", "beta-26.5-2", "beta-26.5-1", "beta-26.5"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v — all 5 beta tags must group into one channel, not fragment into singletons", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSortInChannel_UniformVPrefixAcrossAllTags_StillTreatedAsNoise(t *testing.T) {
	// Regression guard: a set where every tag shares the same bare "v"
	// prefix must classify exactly as it did before this fix — "v" alone
	// is never a channel, regardless of the new prefix-awareness.
	tags := []string{"v1.0.0", "v1.1.0", "v1.2.0-rc1"}

	stable := SortInChannel(tags, "stable")
	wantStable := []string{"v1.1.0", "v1.0.0"}
	if len(stable) != len(wantStable) || stable[0] != wantStable[0] || stable[1] != wantStable[1] {
		t.Errorf("stable = %v, want %v", stable, wantStable)
	}

	rc := SortInChannel(tags, "rc")
	if len(rc) != 1 || rc[0] != "v1.2.0-rc1" {
		t.Errorf("rc = %v, want [v1.2.0-rc1]", rc)
	}
}

func TestSortInChannel_MixedPrefixAndSuffixConventions_PrefixOnlyAppliesWhenNoSuffix(t *testing.T) {
	// A tag with a suffix keeps suffix priority even in a set where prefix
	// variation exists elsewhere.
	tags := []string{"v1.0.0", "v1.1.0-rc1", "beta-2.0"}

	stable := SortInChannel(tags, "stable")
	if len(stable) != 1 || stable[0] != "v1.0.0" {
		t.Errorf("stable = %v, want [v1.0.0]", stable)
	}

	rc := SortInChannel(tags, "rc")
	if len(rc) != 1 || rc[0] != "v1.1.0-rc1" {
		t.Errorf("rc = %v, want [v1.1.0-rc1]", rc)
	}

	beta := SortInChannel(tags, "beta")
	if len(beta) != 1 || beta[0] != "beta-2.0" {
		t.Errorf("beta = %v, want [beta-2.0]", beta)
	}
}
