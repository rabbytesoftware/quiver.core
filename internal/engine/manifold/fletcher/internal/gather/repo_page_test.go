package gather

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePage_Goldens(t *testing.T) {
	testCases := []struct {
		name            string
		file            string
		pageURL         string
		slug            string
		wantDescription string
		wantImage       string
	}{
		{
			name:            "generated social card names the repository",
			file:            "repo_page_ripgrep.html",
			pageURL:         "https://github.com/BurntSushi/ripgrep",
			slug:            "burntsushi/ripgrep",
			wantDescription: "ripgrep recursively searches directories for a regex pattern while respecting your gitignore",
		},
		{
			name:            "custom social image",
			file:            "repo_page_nextjs_custom_image.html",
			pageURL:         "https://github.com/vercel/next.js",
			slug:            "vercel/next.js",
			wantDescription: "The React Framework.",
			wantImage:       "https://repository-images.githubusercontent.com/70107786/4602445c-10a2-4903-a360-c96d70531f67",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := os.ReadFile("testdata/" + tc.file)
			require.NoError(t, err)

			page := parsePage(body, tc.pageURL, tc.slug)

			assert.Equal(t, tc.wantDescription, page.description)
			assert.Equal(t, tc.wantImage, page.socialImage)
		})
	}
}

func TestParsePage_ContentBeforePropertyAndFirstTagWins(t *testing.T) {
	body := `<html><head>
		<meta content="A GitLab CLI tool" property="og:description">
		<meta content="later" property="og:description">
		<meta content="/uploads/avatar/1/logo.png" property="og:image">
		<meta name="description" content="ignored">
	</head></html>`

	page := parsePage([]byte(body), "https://gitlab.com/gitlab-org/cli", "gitlab-org/cli")

	assert.Equal(t, "A GitLab CLI tool", page.description)
	assert.Equal(t, "https://gitlab.com/uploads/avatar/1/logo.png", page.socialImage)
}

func TestParsePage_NoOpenGraph(t *testing.T) {
	page := parsePage([]byte("<html><body><p>nothing</p></body></html>"), "https://h.test/u/r", "u/r")

	assert.Equal(t, repoPage{}, page)
}

func TestCleanDescription(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		slug        string
		want        string
	}{
		{
			name:        "trailing slug",
			description: "fast grep - BurntSushi/ripgrep",
			slug:        "burntsushi/ripgrep",
			want:        "fast grep",
		},
		{
			name:        "sentence naming the slug after a description",
			description: "The lazier way to manage everything docker. Contribute to jesseduffield/lazydocker development by creating an account on GitHub.",
			slug:        "jesseduffield/lazydocker",
			want:        "The lazier way to manage everything docker.",
		},
		{
			name:        "only a sentence naming the slug",
			description: "Contribute to jesseduffield/lazydocker development by creating an account on GitHub.",
			slug:        "jesseduffield/lazydocker",
			want:        "",
		},
		{
			name:        "slug in the middle sentence",
			description: "Fast. Mirror of acme/tool! Works offline?  Yes",
			slug:        "acme/tool",
			want:        "Fast. Works offline? Yes",
		},
		{
			name:        "another repository's suffix stays",
			description: "a fork of the tool - other/tool",
			slug:        "me/tool",
			want:        "a fork of the tool - other/tool",
		},
		{
			name:        "plain description keeps its spacing",
			description: "  A tool.  It does things.  ",
			slug:        "acme/tool",
			want:        "A tool.  It does things.",
		},
		{name: "empty", slug: "acme/tool"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cleanDescription(tc.description, tc.slug))
		})
	}
}

func TestAuthoredImage(t *testing.T) {
	testCases := []struct {
		name    string
		image   string
		pageURL string
		want    string
	}{
		{name: "absolute", image: "https://img.test/a.png", pageURL: "https://h.test/u/r", want: "https://img.test/a.png"},
		{name: "relative", image: "/a.png", pageURL: "https://h.test/u/r", want: "https://h.test/a.png"},
		{name: "names the repository", image: "https://cards.test/x/U/R", pageURL: "https://h.test/u/r"},
		{name: "empty", pageURL: "https://h.test/u/r"},
		{name: "unparseable page url", image: "/a.png", pageURL: "://bad"},
		{name: "unparseable image", image: "://bad", pageURL: "https://h.test/u/r"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, authoredImage(tc.image, tc.pageURL, "u/r"))
		})
	}
}
