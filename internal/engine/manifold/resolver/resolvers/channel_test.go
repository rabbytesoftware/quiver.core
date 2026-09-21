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
