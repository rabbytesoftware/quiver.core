package collection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/aerrors"
	quiverrules "github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset/collection"
)

func TestCheckDuplicateNamespaces_NoDuplicates(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/tool", IsLocal: false},
		{Namespace: "github.com/b/tool", IsLocal: false},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	assert.NoError(t, err)
}

func TestCheckDuplicateNamespaces_DetectsDuplicate(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/tool", IsLocal: false},
		{Namespace: "github.com/a/tool", IsLocal: false},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github.com/a/tool")

	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
}

func TestCheckDuplicateNamespaces_EmptyList(t *testing.T) {
	err := quiverrules.CheckDuplicateNamespaces([]domain.CollectionArrow{})
	assert.NoError(t, err)
}

func TestCheckDuplicateNamespaces_SingleArrow(t *testing.T) {
	arrows := []domain.CollectionArrow{
		{Namespace: "github.com/a/tool", IsLocal: false},
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	assert.NoError(t, err)
}

func TestCheckDuplicateNamespaces_MultipleDuplicates(t *testing.T) {
	ns1 := domain.Namespace("owner/repo@v1/arrow-a")
	ns2 := domain.Namespace("owner/repo@v1/arrow-b")
	arrows := []domain.CollectionArrow{
		{Namespace: ns1, IsLocal: false},
		{Namespace: ns2, IsLocal: false},
		{Namespace: ns1, IsLocal: false}, // dup
		{Namespace: ns2, IsLocal: false}, // dup
	}
	err := quiverrules.CheckDuplicateNamespaces(arrows)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Len(t, ruleErrs, 2, "expected both duplicates reported")
}
