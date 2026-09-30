package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileURLs_Pinned_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name    string
		host    *stubHost
		wantURL string
		wantOK  bool
	}{
		{name: "github", host: &stubHost{}, wantURL: githubRawPrefix + "logo.svg", wantOK: true},
		{name: "gitlab", host: &stubHost{rawTemplate: "https://gitlab.com/owner/repo/-/raw/{branch}/{file}"}, wantURL: "https://gitlab.com/owner/repo/-/raw/v1.0.0/logo.svg", wantOK: true},
		{name: "host without raw urls", host: &stubHost{noTemplates: true}},
		{name: "template without a file slot", host: &stubHost{rawTemplate: "https://x.test/static"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			urls := fileURLsOf(tc.host, testNS, testRef)

			got, ok := urls.pinned("logo.svg")

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantURL, got)
		})
	}
}
