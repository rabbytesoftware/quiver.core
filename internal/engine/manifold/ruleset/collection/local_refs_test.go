package collection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
	quiverrules "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/collection"
)

func TestCheckLocalRefs_LocalArrowWithRef(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/quiver/cs2@v1.0.0", IsLocal: true},
	}
	assert.NoError(t, quiverrules.CheckLocalRefs(arrows))
}

func TestCheckLocalRefs_LocalArrowWithoutRef(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/quiver/cs2", IsLocal: true},
	}

	err := quiverrules.CheckLocalRefs(arrows)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.com/a/quiver/cs2")

	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	require.Len(t, ruleErrs, 1)
	assert.Equal(t, "arrows[0]", ruleErrs[0].Field)
	assert.Equal(t, "required_ref", ruleErrs[0].Rule)
}

// A remote entry resolves its own ref when it is followed, so a refless one is
// not the collection's problem to solve.
func TestCheckLocalRefs_RemoteArrowWithoutRefIsFine(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/other/pkg", IsLocal: false},
	}
	assert.NoError(t, quiverrules.CheckLocalRefs(arrows))
}

func TestCheckLocalRefs_ReportsEveryOffendingEntry(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/quiver/cs2", IsLocal: true},
		{Namespace: "github.com/a/quiver/tf2@v2", IsLocal: true},
		{Namespace: "github.com/a/quiver/hl2", IsLocal: true},
	}

	err := quiverrules.CheckLocalRefs(arrows)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	require.Len(t, ruleErrs, 2)
	assert.Equal(t, "arrows[0]", ruleErrs[0].Field)
	assert.Equal(t, "arrows[2]", ruleErrs[1].Field)
}

func TestCheckLocalRefs_EmptyList(t *testing.T) {
	assert.NoError(t, quiverrules.CheckLocalRefs(nil))
}
