package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoPage_GoldenRipgrep_GeneratedSocialImage(t *testing.T) {
	page, err := parseRepoPage(readTestdata(t, "repo_page_ripgrep.html"), "burntsushi/ripgrep")
	require.NoError(t, err)

	assert.Equal(t, "ripgrep recursively searches directories for a regex pattern while respecting your gitignore", page.Description)
	assert.Equal(
		t,
		"https://opengraph.githubassets.com/3e15eb61bc0afd8ed03e62e06a253d7eed10cbdaa4aa8dde5ba641937dbdf7ad/BurntSushi/ripgrep",
		page.SocialImage,
	)
	assert.False(t, page.CustomSocialImage)
}

func TestParseRepoPage_GoldenNextjs_CustomSocialImage(t *testing.T) {
	page, err := parseRepoPage(readTestdata(t, "repo_page_nextjs_custom_image.html"), "vercel/next.js")
	require.NoError(t, err)

	assert.Equal(t, "The React Framework.", page.Description)
	assert.Equal(
		t,
		"https://repository-images.githubusercontent.com/70107786/4602445c-10a2-4903-a360-c96d70531f67",
		page.SocialImage,
	)
	assert.True(t, page.CustomSocialImage)
}

func TestParseRepoPage_MalformedPage_ReturnsErrUnexpectedPage(t *testing.T) {
	body := `<html><head></head><body><p>nothing here</p></body></html>`

	_, err := parseRepoPage([]byte(body), "u/r")
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}

func TestParseRepoPage_NoSocialImage_CustomSocialImageIsFalse(t *testing.T) {
	body := `<html><head>
		<meta property="og:description" content="a repo with no image" />
	</head><body></body></html>`

	page, err := parseRepoPage([]byte(body), "u/r")
	require.NoError(t, err)
	assert.Equal(t, "a repo with no image", page.Description)
	assert.Empty(t, page.SocialImage)
	assert.False(t, page.CustomSocialImage)
}

func TestIsGeneratedSocialImage_TableDriven(t *testing.T) {
	testCases := []struct {
		name string
		url  string
		want bool
	}{
		{name: "generated", url: "https://opengraph.githubassets.com/abc/u/r", want: true},
		{name: "custom", url: "https://repository-images.githubusercontent.com/1/2", want: false},
		{name: "unparseable", url: "://not a url", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isGeneratedSocialImage(tc.url))
		})
	}
}

func TestCleanDescription(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		slug        string
		want        string
	}{
		{
			name:        "trailing owner and repo",
			description: "ripgrep recursively searches directories for a regex pattern while respecting your gitignore - BurntSushi/ripgrep",
			slug:        "BurntSushi/ripgrep",
			want:        "ripgrep recursively searches directories for a regex pattern while respecting your gitignore",
		},
		{
			name:        "trailing owner and repo in another case",
			description: "fast grep - BurntSushi/ripgrep",
			slug:        "burntsushi/ripgrep",
			want:        "fast grep",
		},
		{
			name:        "contribute sentence after a description",
			description: "The lazier way to manage everything docker. Contribute to jesseduffield/lazydocker development by creating an account on GitHub.",
			slug:        "jesseduffield/lazydocker",
			want:        "The lazier way to manage everything docker.",
		},
		{
			name:        "contribute sentence alone",
			description: "Contribute to jesseduffield/lazydocker development by creating an account on GitHub.",
			slug:        "jesseduffield/lazydocker",
			want:        "",
		},
		{
			name:        "another repository's suffix stays",
			description: "a fork of the tool - other/tool",
			slug:        "me/tool",
			want:        "a fork of the tool - other/tool",
		},
		{
			name:        "plain description is untouched",
			description: "  A tool that does things.  ",
			slug:        "acme/tool",
			want:        "A tool that does things.",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cleanDescription(tc.description, tc.slug))
		})
	}
}
