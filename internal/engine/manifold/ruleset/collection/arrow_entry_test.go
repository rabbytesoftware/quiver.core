package collection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/aerrors"
	quiverrules "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset/collection"
)

func TestCheckArrowEntries_PathOnly_NoError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Path: "servers/cs2"}}
	assert.NoError(t, quiverrules.CheckArrowEntries(entries))
}

func TestCheckArrowEntries_NamespaceOnly_NoError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Namespace: "github.com/other/tool"}}
	assert.NoError(t, quiverrules.CheckArrowEntries(entries))
}

func TestCheckArrowEntries_BothPathAndNamespace_ReturnsError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Path: "servers/cs2", Namespace: "github.com/other/tool"}}
	err := quiverrules.CheckArrowEntries(entries)
	require.Error(t, err)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Equal(t, "exclusive_fields", ruleErrs[0].Rule)
}

func TestCheckArrowEntries_NeitherPathNorNamespace_ReturnsError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{}}
	err := quiverrules.CheckArrowEntries(entries)
	require.Error(t, err)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Equal(t, "required_field", ruleErrs[0].Rule)
}

func TestCheckArrowEntries_AUIDWithPath_NoError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{
		{Path: "tools/legacy/appimage-runtime", AUID: "appimage-runtime"},
	}
	assert.NoError(t, quiverrules.CheckArrowEntries(entries))
}

func TestCheckArrowEntries_AUIDWithNamespace_ReturnsError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{
		{Namespace: "github.com/other/tool", AUID: "renamed"},
	}
	err := quiverrules.CheckArrowEntries(entries)
	require.Error(t, err)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Equal(t, "auid_with_namespace", ruleErrs[0].Rule)
}

func TestCheckArrowEntries_AUIDContainsSlash_ReturnsError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Path: "tools/x", AUID: "tools/x"}}
	err := quiverrules.CheckArrowEntries(entries)
	require.Error(t, err)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Equal(t, "invalid_auid", ruleErrs[0].Rule)
}

func TestCheckArrowEntries_AUIDContainsAt_ReturnsError(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Path: "tools/x", AUID: "x@v1"}}
	err := quiverrules.CheckArrowEntries(entries)
	require.Error(t, err)
	var ruleErrs aerrors.RuleErrors
	require.ErrorAs(t, err, &ruleErrs)
	assert.Equal(t, "invalid_auid", ruleErrs[0].Rule)
}

func TestCheckArrowEntries_EmptyAUID_TreatedAsUnset(t *testing.T) {
	entries := []domain.CollectionArrowEntry{{Path: "servers/cs2", AUID: ""}}
	assert.NoError(t, quiverrules.CheckArrowEntries(entries))
}
