package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

func TestChannelListDTOFrom_MapsEveryField(t *testing.T) {
	channels := []models.ChannelInfo{
		{
			Name:    "rc",
			Kind:    "ordered",
			Latest:  "v1.5.0-rc2",
			Count:   2,
			Members: []string{"v1.5.0-rc2", "v1.5.0-rc1"},
		},
		{
			Name:   "main",
			Kind:   "pointer",
			Latest: "main",
		},
	}

	got := dto.ChannelListDTOFrom(channels)
	require.Len(t, got.Channels, 2)

	rc := got.Channels[0]
	assert.Equal(t, "rc", rc.Name)
	assert.Equal(t, "ordered", rc.Kind)
	assert.Equal(t, "v1.5.0-rc2", rc.Latest)
	assert.Equal(t, 2, rc.Count)
	assert.Equal(t, []string{"v1.5.0-rc2", "v1.5.0-rc1"}, rc.Members)

	main := got.Channels[1]
	assert.Equal(t, "main", main.Name)
	assert.Equal(t, "pointer", main.Kind)
	assert.Equal(t, "main", main.Latest)
	assert.Empty(t, main.Members)
}

func TestChannelListDTOFrom_Empty(t *testing.T) {
	got := dto.ChannelListDTOFrom(nil)
	require.NotNil(t, got.Channels)
	assert.Empty(t, got.Channels)
}
