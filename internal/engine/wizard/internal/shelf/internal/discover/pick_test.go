package discover

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/shelf/mocks"
)

func TestIcon(t *testing.T) {
	wd := filepath.Join(t.TempDir(), "wd")

	testCases := []struct {
		name      string
		entry     string
		candidate string
		media     string
		want      string
	}{
		{name: "entry icon", entry: "${INSTALL_PATH}/icon.png", candidate: "/c.png", media: "/m.png", want: filepath.Join(wd, "icon.png")},
		{name: "candidate icon over media", candidate: "/c.png", media: "/m.png", want: "/c.png"},
		{name: "media path", media: "/m.png", want: "/m.png"},
		{name: "media url", media: "https://raw.example/icon.png", want: ""},
		{name: "control characters", media: "/m\n.png", want: ""},
		{name: "none", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := models.Request{Workdir: wd, Media: domain.ArrowMedia{Icon: tc.media}}
			assert.Equal(t, tc.want, Icon(req, domain.ExposeEntry{Icon: tc.entry}, models.Candidate{Icon: tc.candidate}))
		})
	}
}

func TestPick(t *testing.T) {
	one := models.Candidate{Name: "One", Depth: 1}
	tool := models.Candidate{Name: "tool", Depth: 2}
	shallowTool := models.Candidate{Name: "Tool", Depth: 1}

	testCases := []struct {
		name       string
		found      []models.Candidate
		entryName  string
		want       models.Candidate
		wantReason string
	}{
		{name: "nothing", wantReason: models.ReasonNoDesktop},
		{name: "single", found: []models.Candidate{one}, want: one},
		{name: "repo name, shallowest first", found: []models.Candidate{one, tool, shallowTool}, want: shallowTool},
		{name: "entry name", found: []models.Candidate{tool, one}, entryName: "one", want: one},
		{name: "no match", found: []models.Candidate{one, {Name: "Two"}}, wantReason: models.ReasonAmbiguous},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := Pick(tc.found, "tool", tc.entryName)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantReason, reason)
		})
	}
}

func TestRenamed(t *testing.T) {
	c := models.Candidate{Name: "setup"}

	assert.Equal(t, "Tool", Renamed(c, "Tool").Name)
	assert.Equal(t, "setup", Renamed(c, "../x").Name)
}

func TestRepoName(t *testing.T) {
	assert.Equal(t, "tool", RepoName(mocks.BareA))
	assert.Equal(t, "r", RepoName("q.io/u/r/auid"))
	assert.Empty(t, RepoName("short/ns"))
}
