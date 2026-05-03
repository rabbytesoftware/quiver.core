package quiver_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
	quiverrules "github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/quiver"
)

func TestCheckDuplicateNamespaces_NoDuplicates(t *testing.T) {
	arrows := []domain.QuiverArrow{
		{Namespace: "github.com/a/tool"},
		{Namespace: "github.com/b/tool"},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	assert.NoError(t, err)
}

func TestCheckDuplicateNamespaces_DetectsDuplicate(t *testing.T) {
	arrows := []domain.QuiverArrow{
		{Namespace: "github.com/a/tool"},
		{Namespace: "github.com/a/tool"},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.com/a/tool")

	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
}

func TestCheckDuplicateNamespaces_EmptyList(t *testing.T) {
	err := quiverrules.CheckDuplicateNamespaces([]domain.QuiverArrow{})
	assert.NoError(t, err)
}

func TestCheckDuplicateNamespaces_SingleArrow(t *testing.T) {
	arrows := []domain.QuiverArrow{
		{Namespace: "github.com/a/tool"},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	assert.NoError(t, err)
}
