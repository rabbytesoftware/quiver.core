package commands

import "testing"

func TestMatches_GlobCrossesSlash(t *testing.T) {
	ns := "github.com/user/quiver.arrow-discord"

	testCases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"match all", "*", true},
		{"surrounding wildcards", "*discord*", true},
		{"leading wildcard across slashes", "*/quiver.arrow-discord", true},
		{"trailing wildcard on name", "discord*", true},
		{"exact namespace", ns, true},
		{"single-segment wildcard", "github.com/user/*", true},
		{"substring fallback", "discord", true},
		{"empty matches all", "", true},
		{"no match", "nginx", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := matches(tc.pattern, ns, "discord")
			if got != tc.want {
				t.Fatalf("matches(%q, %q, %q) = %v, want %v", tc.pattern, ns, "discord", got, tc.want)
			}
		})
	}
}
