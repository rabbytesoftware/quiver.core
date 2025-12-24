package arrows

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewArrowsRepository(t *testing.T) {
	repo := NewArrowsRepository()
	require.NotNil(t, repo)
}

func TestArrowsRepository_Get(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	namespace := shared.Namespace("test:arrow")

	arrow, errs, err := repo.Get(ctx, namespace)
	assert.NoError(t, err)
	assert.Nil(t, errs)
	assert.NotNil(t, arrow)
}

func TestArrowsRepository_List(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	arrows, errs, err := repo.List(ctx)
	assert.NoError(t, err)
	assert.Nil(t, errs)
	assert.Nil(t, arrows)
}

func TestArrowsRepository_Add(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	namespace := shared.Namespace("test:arrow")

	arrow, errs, err := repo.Add(ctx, namespace, "/path/to/arrow", false, "127.0.0.1")
	assert.NoError(t, err)
	assert.Nil(t, errs)
	assert.NotNil(t, arrow)
}

func TestArrowsRepository_Remove(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	namespace := shared.Namespace("test:arrow")

	errs, err := repo.Remove(ctx, namespace, false, "127.0.0.1")
	assert.NoError(t, err)
	assert.Nil(t, errs)
}

func TestArrowsRepository_ExecuteMethod(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	namespace := shared.Namespace("test:arrow")

	variables := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	errs, err := repo.ExecuteMethod(ctx, namespace, "start", variables, "127.0.0.1")
	assert.NoError(t, err)
	assert.Nil(t, errs)
}

func TestArrowsRepository_StopMethod(t *testing.T) {
	repo := NewArrowsRepository()
	ctx := context.Background()

	namespace := shared.Namespace("test:arrow")

	errs, err := repo.StopMethod(ctx, namespace, "start")
	assert.NoError(t, err)
	assert.Nil(t, errs)
}
