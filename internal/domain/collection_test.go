package domain_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestCollectionArrowEntry_MutuallyExclusive(t *testing.T) {
	local := domain.CollectionArrowEntry{Path: "servers/cs2"}
	ext := domain.CollectionArrowEntry{Namespace: "github.com/valve/steamcmd"}
	assert.NotEmpty(t, local.Path)
	assert.NotEmpty(t, ext.Namespace)
}

func TestQuiver_HasFollowedAt(t *testing.T) {
	q := domain.Collection{
		Namespace:  domain.Namespace("github.com/char2cs/gaming.quiver"),
		FollowedAt: time.Now(),
	}
	assert.False(t, q.FollowedAt.IsZero())
}

// A collection is a list of arrows that each carry their own ref. The list itself
// names no artifact, so a version on it identifies nothing and resolves against
// nothing. The metadata block must not declare one, and the aggregate — persisted
// verbatim through asynx and the vault envelope — must not encode one.
func TestCollectionMeta_HasNoVersionField(t *testing.T) {
	_, found := reflect.TypeOf(domain.CollectionMeta{}).FieldByName("Version")
	assert.False(t, found, "CollectionMeta must not declare a Version field")

	blob, err := json.Marshal(domain.Collection{
		Namespace: domain.Namespace("github.com/char2cs/gaming.quiver@v1.0.0"),
		Meta:      domain.CollectionMeta{Name: "Gaming Quiver", Description: "curated"},
		Arrows:    []domain.CollectionArrow{{Namespace: "github.com/valve/steamcmd@v2", IsLocal: false}},
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(blob, &decoded))

	meta, ok := decoded["Meta"].(map[string]any)
	require.True(t, ok, "encoded collection = %s, want a Meta object", blob)
	assert.NotContains(t, meta, "Version")
	assert.Equal(t, "Gaming Quiver", meta["Name"])
}

// Events and cached envelopes written before the field was removed still carry
// the key. Asynx and the vault both decode with encoding/json, which ignores
// unknown keys, so replay must survive the old shape without an upcaster.
func TestCollection_UnmarshalsLegacyValueWithMetaVersion(t *testing.T) {
	legacy := []byte(`{
		"Namespace": "github.com/char2cs/gaming.quiver@v1.0.0",
		"Meta": {"Name": "Gaming Quiver", "Version": "1.0.0", "Description": "curated"},
		"Arrows": [{"Namespace": "github.com/valve/steamcmd@v2", "IsLocal": false}]
	}`)

	var coll domain.Collection
	require.NoError(t, json.Unmarshal(legacy, &coll))

	assert.Equal(t, "Gaming Quiver", coll.Meta.Name)
	assert.Equal(t, "curated", coll.Meta.Description)
	require.Len(t, coll.Arrows, 1)
	assert.Equal(t, "v2", coll.Arrows[0].Namespace.Ref())
}
