package store

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestArrow(ns string, name string) domain.Arrow {
	return domain.Arrow{
		Namespace: domain.Namespace(ns),
		Manifest: domain.ArrowManifest{
			Name:    name,
			Version: "1.0.0",
		},
		Removed: false,
	}
}

func TestArrowCatalog_SaveAndGet_ReturnsSavedArrow(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	err = c.Save(arrow)
	require.NoError(t, err)

	got, err := c.Get(arrow.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, arrow.Namespace, got.Namespace)
	assert.Equal(t, arrow.Manifest.Name, got.Manifest.Name)
	assert.Equal(t, arrow.Manifest.Version, got.Manifest.Version)
	assert.Equal(t, arrow.Removed, got.Removed)
}

func TestArrowCatalog_SaveDeleteGet_ReturnsNil(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	err = c.Save(arrow)
	require.NoError(t, err)

	err = c.Delete(arrow.Namespace)
	require.NoError(t, err)

	got, err := c.Get(arrow.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestArrowCatalog_List_ReturnsAllSaved(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	a1 := makeTestArrow("github.com/org/repo1", "Arrow1")
	a2 := makeTestArrow("github.com/org/repo2", "Arrow2")

	require.NoError(t, c.Save(a1))
	require.NoError(t, c.Save(a2))

	arrows, err := c.List()
	require.NoError(t, err)
	assert.Len(t, arrows, 2)

	namespaces := make([]domain.Namespace, 0, len(arrows))
	for _, a := range arrows {
		namespaces = append(namespaces, a.Namespace)
	}
	assert.Contains(t, namespaces, a1.Namespace)
	assert.Contains(t, namespaces, a2.Namespace)
}

func TestArrowCatalog_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	require.NoError(t, c.Save(arrow))
	require.NoError(t, c.Delete(arrow.Namespace))

	arrows, err := c.List()
	require.NoError(t, err)
	assert.Empty(t, arrows)
}
