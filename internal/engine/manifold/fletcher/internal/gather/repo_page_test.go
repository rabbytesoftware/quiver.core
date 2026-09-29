package gather

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePage_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name            string
		body            string
		pageURL         string
		slug            string
		wantDescription string
		wantImage       string
	}{
		{
			name: "generated social card names the repository",
			body: `<meta property="og:image" content="https://opengraph.githubassets.com/3e15/BurntSushi/ripgrep" />` +
				`<meta property="og:description" content="ripgrep recursively searches directories - BurntSushi/ripgrep" />`,
			pageURL:         "https://github.com/BurntSushi/ripgrep",
			slug:            "burntsushi/ripgrep",
			wantDescription: "ripgrep recursively searches directories",
		},
		{
			name: "custom social image",
			body: `<meta property="og:image" content="https://repository-images.githubusercontent.com/70107786/4602445c" />` +
				`<meta property="og:description" content="The React Framework. Contribute to vercel/next.js development by creating an account on GitHub." />`,
			pageURL:         "https://github.com/vercel/next.js",
			slug:            "vercel/next.js",
			wantDescription: "The React Framework.",
			wantImage:       "https://repository-images.githubusercontent.com/70107786/4602445c",
		},
		{
			name: "content before property and first tag wins",
			body: `<meta content="A GitLab CLI tool" property="og:description">` +
				`<meta content="later" property="og:description">` +
				`<meta content="/uploads/avatar/1/logo.png" property="og:image">` +
				`<meta name="description" content="ignored">`,
			pageURL:         "https://gitlab.com/gitlab-org/cli",
			slug:            "gitlab-org/cli",
			wantDescription: "A GitLab CLI tool",
			wantImage:       "https://gitlab.com/uploads/avatar/1/logo.png",
		},
		{
			name:    "no open graph",
			body:    "<p>nothing</p>",
			pageURL: "https://h.test/u/r",
			slug:    "u/r",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			page := parsePage([]byte("<html><head>"+tc.body+"</head></html>"), tc.pageURL, tc.slug)

			assert.Equal(t, tc.wantDescription, page.description)
			assert.Equal(t, tc.wantImage, page.socialImage)
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
