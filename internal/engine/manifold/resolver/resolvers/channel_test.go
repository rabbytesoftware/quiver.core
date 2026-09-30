package resolvers

import "testing"

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

func TestSortInChannel_OrdersByPrecedence(t *testing.T) {
	testCases := []struct {
		name    string
		tags    []string
		channel string
		want    []string
	}{
		{
			name:    "rc ordinals across releases, highest first",
			tags:    []string{"v1.2.0-rc1", "v1.2.0-rc2", "v1.3.0-rc1"},
			channel: "rc",
			want:    []string{"v1.3.0-rc1", "v1.2.0-rc2", "v1.2.0-rc1"},
		},
		{
			name:    "no tag in the requested channel",
			tags:    []string{"v1.4.0"},
			channel: "beta",
			want:    nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := SortInChannel(tc.tags, tc.channel)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
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

func TestClassifyOne_PureNumericSuffixWithoutMeaningfulPrefix_PreservesOrdinal(t *testing.T) {
	// A bare numeric suffix (e.g. "1" in "v1.2.0-1") still names a specific
	// build even when there's no meaningful prefix to attach it to as a
	// channel — it must fall back into "stable" without losing its ordinal.
	channel, ordinal := classifyOne("1", "", false)
	if channel != StableChannel {
		t.Errorf("channel = %q, want %q", channel, StableChannel)
	}
	if ordinal != 1 {
		t.Errorf("ordinal = %d, want 1 — must not be silently zeroed in the stable fallback", ordinal)
	}
}

func TestSortInChannel_UniformPrefixWithNumericOrdinalSuffix_BreaksTiesWithinStable(t *testing.T) {
	// Regression guard for the "stable fallback discards the ordinal" bug: a
	// uniform "v" prefix carries no channel signal (see
	// TestSortInChannel_UniformVPrefixAcrossAllTags_StillTreatedAsNoise), but
	// a bare numeric suffix on one of two same-core tags must still break
	// their tie within the "stable" channel both fall back to.
	tags := []string{"v1.2.0", "v1.2.0-1"}

	got := SortInChannel(tags, "stable")
	want := []string{"v1.2.0-1", "v1.2.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGroupChannels_EqualRankTiesAreTotal(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		want []TagChannel
	}{
		{
			name: "v1.2 and v1.2.0 rank equal; the name breaks the tie",
			tags: []string{"v1.2", "v1.1.0", "v1.2.0"},
			want: []TagChannel{{Name: "stable", Members: []string{"v1.2.0", "v1.2", "v1.1.0"}}},
		},
		{
			name: "1.2.0 and v1.2.0 rank equal; the name breaks the tie",
			tags: []string{"1.2.0", "v1.2.0"},
			want: []TagChannel{{Name: "stable", Members: []string{"v1.2.0", "1.2.0"}}},
		},
		{
			name: "channels listed alphabetically, each in precedence order",
			tags: []string{"v1.0.0", "v2.0.0-rc1", "v2.0.0-beta.2", "v2.0.0-beta.1"},
			want: []TagChannel{
				{Name: "beta", Members: []string{"v2.0.0-beta.2", "v2.0.0-beta.1"}},
				{Name: "rc", Members: []string{"v2.0.0-rc1"}},
				{Name: StableChannel, Members: []string{"v1.0.0"}},
			},
		},
		{
			name: "no versioned tag has no ordered channel",
			tags: []string{"nightly"},
			want: []TagChannel{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range [][]string{tc.tags, reversed(tc.tags)} {
				got := GroupChannels(order)
				if !equalGroups(got, tc.want) {
					t.Errorf("GroupChannels(%v) = %v, want %v", order, got, tc.want)
				}
			}
		})
	}
}

func reversed(tags []string) []string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		out[len(tags)-1-i] = tag
	}
	return out
}

func equalGroups(a, b []TagChannel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || len(a[i].Members) != len(b[i].Members) {
			return false
		}
		for j := range a[i].Members {
			if a[i].Members[j] != b[i].Members[j] {
				return false
			}
		}
	}
	return true
}

func TestGroupChannels_ChannelNamedPrefixesAndDates(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		want []TagChannel
	}{
		{
			name: "a uniform known prefix still names its channel",
			tags: []string{"beta-1.0", "beta-1.1"},
			want: []TagChannel{{Name: "beta", Members: []string{"beta-1.1", "beta-1.0"}}},
		},
		{
			name: "a known prefix does not make an unknown uniform prefix count",
			tags: []string{"release-1.0", "release-1.1", "beta-2.0"},
			want: []TagChannel{
				{Name: "beta", Members: []string{"beta-2.0"}},
				{Name: StableChannel, Members: []string{"release-1.1", "release-1.0"}},
			},
		},
		{
			name: "a date core ranks by year within the century, then month and day",
			tags: []string{"beta-26.5-4", "beta-2026-09-27", "beta-26.10", "beta-2026-09-27-1"},
			want: []TagChannel{{Name: "beta", Members: []string{"beta-26.10", "beta-2026-09-27-1", "beta-2026-09-27", "beta-26.5-4"}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GroupChannels(tc.tags)
			if !equalGroups(got, tc.want) {
				t.Errorf("GroupChannels(%v) = %v, want %v", tc.tags, got, tc.want)
			}
		})
	}
}

func TestOutranks(t *testing.T) {
	testCases := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "higher core", a: "v1.3.0", b: "v1.2.0", want: true},
		{name: "lower core", a: "v1.2.0", b: "v1.3.0", want: false},
		{name: "date after calendar month", a: "stable-2026-09-27", b: "stable-26.5.1", want: true},
		{name: "calendar month after date", a: "stable-26.10", b: "stable-2026-09-27", want: true},
		{name: "higher ordinal on one core", a: "beta-26.5-4", b: "beta-26.5-3", want: true},
		{name: "equal rank", a: "v1.2", b: "v1.2.0", want: false},
		{name: "no core on a", a: "nightly", b: "v1.0.0", want: false},
		{name: "no core on b", a: "v1.0.0", b: "nightly", want: false},
		{name: "a pre-2000 year is kept whole", a: "1999-01-01", b: "v26.0", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Outranks(tc.a, tc.b); got != tc.want {
				t.Errorf("Outranks(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
