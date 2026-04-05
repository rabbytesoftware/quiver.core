package store

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestQuiver(ns string, name string) domain.Quiver {
	return domain.Quiver{
		Namespace: domain.Namespace(ns),
		Manifest: domain.QuiverManifest{
			Name:        name,
			Description: "A test quiver",
			Tags:        []string{"test"},
		},
		Removed: false,
	}
}

func TestQuiverCatalog_SaveAndGet_ReturnsSavedQuiver(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	err = c.Save(quiver)
	require.NoError(t, err)

	got, err := c.Get(quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, quiver.Namespace, got.Namespace)
	assert.Equal(t, quiver.Manifest.Name, got.Manifest.Name)
	assert.Equal(t, quiver.Manifest.Description, got.Manifest.Description)
	assert.Equal(t, quiver.Removed, got.Removed)
}

func TestQuiverCatalog_SaveDeleteGet_ReturnsNil(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	err = c.Save(quiver)
	require.NoError(t, err)

	err = c.Delete(quiver.Namespace)
	require.NoError(t, err)

	got, err := c.Get(quiver.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverCatalog_List_ReturnsAllSaved(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	q1 := makeTestQuiver("github.com/org/repo1", "Quiver1")
	q2 := makeTestQuiver("github.com/org/repo2", "Quiver2")

	require.NoError(t, c.Save(q1))
	require.NoError(t, c.Save(q2))

	quivers, err := c.List()
	require.NoError(t, err)
	assert.Len(t, quivers, 2)

	namespaces := make([]domain.Namespace, 0, len(quivers))
	for _, q := range quivers {
		namespaces = append(namespaces, q.Namespace)
	}
	assert.Contains(t, namespaces, q1.Namespace)
	assert.Contains(t, namespaces, q2.Namespace)
}

func TestQuiverCatalog_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	require.NoError(t, c.Save(quiver))
	require.NoError(t, c.Delete(quiver.Namespace))

	quivers, err := c.List()
	require.NoError(t, err)
	assert.Empty(t, quivers)
}

func TestQuiverCatalog_GetNonExistent_ReturnsNil(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	got, err := c.Get("github.com/org/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverCatalog_Save_UpdatesExisting(t *testing.T) {
	c, err := NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")
	require.NoError(t, c.Save(quiver))

	quiver.Manifest.Name = "UpdatedQuiver"
	require.NoError(t, c.Save(quiver))

	got, err := c.Get(quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "UpdatedQuiver", got.Manifest.Name)
}
