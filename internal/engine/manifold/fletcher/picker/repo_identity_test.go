package picker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPicker_Identify_Repos(t *testing.T) {
	testCases := []struct {
		name string
		repo string
		want repoIdentity
	}{
		{name: "owner and repo", repo: "Byron/dua-cli", want: repoIdentity{name: "dua-cli", product: "duacli", tokens: []string{"dua", "cli"}}},
		{name: "no owner", repo: "Tool", want: repoIdentity{name: "tool", product: "tool", tokens: []string{"tool"}}},
		{name: "symbols only", repo: "u/--", want: repoIdentity{name: "--", product: "", tokens: []string{"", ""}}},
	}

	p := New().(*picker)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, p.identify(tc.repo))
		})
	}
}

func TestRepoIdentity_Accepts_Candidates(t *testing.T) {
	testCases := []struct {
		name  string
		id    repoIdentity
		class classification
		want  bool
	}{
		{name: "plain product", id: repoIdentity{tokens: []string{"tool"}}, class: classification{product: "tool"}, want: true},
		{name: "foreign companion", id: repoIdentity{tokens: []string{"tool"}}, class: classification{product: "toolcli", companions: []string{"cli"}}},
		{name: "companion in repo name", id: repoIdentity{tokens: []string{"tool", "cli"}}, class: classification{product: "toolcli", companions: []string{"cli"}}, want: true},
		{name: "one foreign among many", id: repoIdentity{tokens: []string{"tool", "cli"}}, class: classification{product: "toolcliserver", companions: []string{"cli", "server"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.accepts(tc.class))
		})
	}
}

func TestRepoIdentity_Owns_Products(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    bool
	}{
		{name: "equal", id: repoIdentity{product: "crowbar"}, product: "crowbar", want: true},
		{name: "suffixed", id: repoIdentity{product: "crowbar"}, product: "crowbarapi"},
		{name: "empty identity", id: repoIdentity{}, product: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.owns(classification{product: tc.product}))
		})
	}
}

func TestRepoIdentity_Extends_Products(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    bool
	}{
		{name: "equal", id: repoIdentity{product: "tool"}, product: "tool", want: true},
		{name: "suffixed", id: repoIdentity{product: "tool"}, product: "toolkit", want: true},
		{name: "shorter", id: repoIdentity{product: "toolcli"}, product: "tool"},
		{name: "unrelated", id: repoIdentity{product: "pake"}, product: "grok"},
		{name: "empty identity", id: repoIdentity{}, product: "tool"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.extends(classification{product: tc.product}))
		})
	}
}

func TestRepoIdentity_MentionedIn_Names(t *testing.T) {
	testCases := []struct {
		name  string
		id    repoIdentity
		asset string
		want  bool
	}{
		{name: "prefix", id: repoIdentity{name: "zap"}, asset: "Zap-linux.tar.gz", want: true},
		{name: "inside", id: repoIdentity{name: "zap"}, asset: "go-zap-linux.tar.gz", want: true},
		{name: "absent", id: repoIdentity{name: "zap"}, asset: "other-linux.tar.gz"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.mentionedIn(classification{asset: asset(tc.asset)}))
		})
	}
}

func TestRepoIdentity_Extras_Counts(t *testing.T) {
	testCases := []struct {
		name    string
		id      repoIdentity
		product string
		want    int
	}{
		{name: "equal", id: repoIdentity{product: "tool"}, product: "tool", want: 0},
		{name: "suffix", id: repoIdentity{product: "tool"}, product: "toolserver", want: 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.id.extras(classification{product: tc.product}))
		})
	}
}
