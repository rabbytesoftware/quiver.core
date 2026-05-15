package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestCollectionEventDTOFrom_Upserted(t *testing.T) {
	evt := hub.CollectionEvent{
		Kind: hub.CatalogUpserted,
		Collection: domain.Collection{
			Namespace: "github.com/user/quiver",
			Meta:      domain.CollectionMeta{Name: "quiver", Description: "desc", Tags: []string{"b"}},
		},
	}
	data, err := json.Marshal(dto.CollectionEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "upserted", m["event"])
	assert.Equal(t, "github.com/user/quiver", m["namespace"])
	assert.Equal(t, "quiver", m["name"])
	assert.Equal(t, "desc", m["description"])
}

func TestCollectionEventDTOFrom_Removed(t *testing.T) {
	evt := hub.CollectionEvent{
		Kind:       hub.CatalogRemoved,
		Collection: domain.Collection{Namespace: "github.com/user/quiver"},
	}
	data, err := json.Marshal(dto.CollectionEventDTOFrom(evt))
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	assert.Equal(t, "removed", m["event"])
	assert.Equal(t, "github.com/user/quiver", m["namespace"])
}

func TestQuiverDTOFrom(t *testing.T) {
	q := domain.Collection{
		Namespace: "github.com/user/repo",
	}
	d := dto.QuiverDTOFrom(q)
	assert.Equal(t, "github.com/user/repo", d.Namespace)
}
