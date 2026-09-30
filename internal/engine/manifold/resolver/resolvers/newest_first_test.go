package resolvers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

func TestNewestFirst_HighestVersionCoreLeads(t *testing.T) {
	testCases := []struct {
		name     string
		channels []models.ChannelInfo
		want     []string
	}{
		{
			name: "alphabetical alpha does not beat a newer stable",
			channels: []models.ChannelInfo{
				{Name: "alpha", Kind: "ordered", Latest: "v0.0.4-alpha.1"},
				{Name: "stable", Kind: "ordered", Latest: "v0.0.44"},
			},
			want: []string{"stable", "alpha"},
		},
		{
			name: "numeric not lexical comparison",
			channels: []models.ChannelInfo{
				{Name: "beta", Kind: "ordered", Latest: "v1.9.0-beta.1"},
				{Name: "rc", Kind: "ordered", Latest: "v1.10.0-rc.1"},
			},
			want: []string{"rc", "beta"},
		},
		{
			name: "equal cores keep input order",
			channels: []models.ChannelInfo{
				{Name: "alpha", Kind: "ordered", Latest: "v1.0.0-alpha.1"},
				{Name: "beta", Kind: "ordered", Latest: "v1.0.0-beta.1"},
			},
			want: []string{"alpha", "beta"},
		},
		{
			name: "pointer channels trail ordered ones and keep their order",
			channels: []models.ChannelInfo{
				{Name: "nightly", Kind: "pointer", Latest: "nightly"},
				{Name: "tip", Kind: "pointer", Latest: "tip"},
				{Name: "beta", Kind: "ordered", Latest: "v1.0.0-beta.1"},
			},
			want: []string{"beta", "nightly", "tip"},
		},
		{
			name: "ordered channel without a version core sorts last among ordered",
			channels: []models.ChannelInfo{
				{Name: "odd", Kind: "ordered", Latest: "weird"},
				{Name: "beta", Kind: "ordered", Latest: "v0.1.0-beta.1"},
			},
			want: []string{"beta", "odd"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range NewestFirst(tc.channels) {
				got = append(got, c.Name)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHasVersionCore(t *testing.T) {
	testCases := []struct {
		name string
		tag  string
		want bool
	}{
		{name: "semver", tag: "v1.2.0", want: true},
		{name: "prerelease", tag: "v1.2.0-rc.1", want: true},
		{name: "dated channel", tag: "stable-2026-09-27", want: true},
		{name: "rolling pointer", tag: "tip", want: false},
		{name: "rolling channel word", tag: "nightly-latest", want: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasVersionCore(tc.tag))
		})
	}
}
