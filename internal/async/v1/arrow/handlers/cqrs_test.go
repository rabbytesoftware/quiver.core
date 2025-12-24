package arrow

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewArrowView(t *testing.T) {
	view := NewArrowView()
	require.NotNil(t, view)
}

func TestArrowView_Get(t *testing.T) {
	view := NewArrowView()
	ctx := context.Background()

	result, err := view.Get(ctx, "test-namespace")
	assert.NoError(t, err)
	assert.Equal(t, arrow.Arrow{}, result)
}

func TestArrowView_List(t *testing.T) {
	view := NewArrowView()
	ctx := context.Background()

	result, err := view.List(ctx)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestArrowView_Add(t *testing.T) {
	view := NewArrowView()
	ctx := context.Background()
	testArrow := arrow.Arrow{
		Name:    "test-arrow",
		Version: "1.0.0",
	}

	err := view.Add(ctx, testArrow)
	assert.NoError(t, err)
}

func TestArrowView_Update(t *testing.T) {
	view := NewArrowView()
	ctx := context.Background()
	testArrow := arrow.Arrow{
		Name:    "test-arrow",
		Version: "1.0.1",
	}

	err := view.Update(ctx, testArrow)
	assert.NoError(t, err)
}

func TestArrowView_Remove(t *testing.T) {
	view := NewArrowView()
	ctx := context.Background()
	testArrow := arrow.Arrow{
		Name:    "test-arrow",
		Version: "1.0.0",
	}

	err := view.Remove(ctx, testArrow)
	assert.NoError(t, err)
}
