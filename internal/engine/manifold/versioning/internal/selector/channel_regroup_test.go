package selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

// coreReleaseTags is quiver.core's own tag list on 2026-09-30, before and
// after the beta/2026-09-27 branch is merged to master.
func coreReleaseTags(extra ...string) domain.RefSnapshot {
	tags := map[string]string{
		"stable-26.5": "s265", "stable-26.5.1": "s2651",
		"beta-26.5": "b0", "beta-26.5-1": "b1", "beta-26.5-2": "b2", "beta-26.5-3": "b3", "beta-26.5-4": "b4",
		"hotfix-26.5.2": "h2652", "nightly-latest": "n", "beta-2026-09-27": "bdate",
	}
	for _, tag := range extra {
		tags[tag] = "c-" + tag
	}
	return domain.RefSnapshot{Tags: tags, Branches: map[string]string{"develop": "d", "master": "m"}, Head: "develop"}
}

func TestChannelsOf_DatedTagsGroupUnderTheirChannel(t *testing.T) {
	channels := ChannelsOf(coreReleaseTags("stable-2026-09-27"))

	byName := map[string]models.ChannelInfo{}
	for _, c := range channels {
		byName[c.Name] = c
	}
	require.Contains(t, byName, "beta")
	require.Contains(t, byName, "stable")
	assert.NotContains(t, byName, "beta-2026-09-27", "a dated beta is a beta, not a channel of its own")
	assert.NotContains(t, byName, "stable-2026-09-27")
	assert.Equal(t, "beta-2026-09-27", byName["beta"].Latest)
	assert.Equal(t, "stable-2026-09-27", byName["stable"].Latest)
	assert.Equal(t, []string{"stable-2026-09-27", "stable-26.5.1", "stable-26.5"}, byName["stable"].Members)
	assert.Equal(t, "hotfix-26.5.2", byName["hotfix"].Latest)
	assert.Equal(t, "pointer", byName["nightly-latest"].Kind)
}

// The names release-tag.sh publishes for a dated series (pinned by
// tests/releasetags): patches count .1, .2 after the date, rebuilds -1, -2.
func TestChannelsOf_DatedSeriesAsTheWorkflowsNameIt(t *testing.T) {
	snap := coreReleaseTags("stable-2026-09-27", "stable-2026-09-27.1", "hotfix-2026-09-27.1", "hotfix-2026-09-27.1-1", "beta-2026-09-27-1")

	byName := map[string]models.ChannelInfo{}
	for _, c := range ChannelsOf(snap) {
		byName[c.Name] = c
	}

	assert.ElementsMatch(t, []string{"stable", "beta", "hotfix", "nightly-latest"}, keys(byName), "no bogus channel from a date")
	assert.Equal(t, []string{"stable-2026-09-27.1", "stable-2026-09-27", "stable-26.5.1", "stable-26.5"}, byName["stable"].Members)
	assert.Equal(t, []string{"hotfix-2026-09-27.1-1", "hotfix-2026-09-27.1", "hotfix-26.5.2"}, byName["hotfix"].Members)
	assert.Equal(t, "beta-2026-09-27-1", byName["beta"].Latest)
}

func keys(m map[string]models.ChannelInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
