package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

func TestShutdownDTOFrom(t *testing.T) {
	testCases := []struct {
		name string
		info models.ShutdownInfo
		want dto.ShutdownDTO
	}{
		{
			name: "nil args becomes empty slice",
			info: models.ShutdownInfo{PID: 7, Exe: "/q"},
			want: dto.ShutdownDTO{PID: 7, Exe: "/q", Args: []string{}},
		},
		{
			name: "args are preserved",
			info: models.ShutdownInfo{PID: 7, Exe: "/q", Args: []string{"daemon", "--host", "unix://"}},
			want: dto.ShutdownDTO{PID: 7, Exe: "/q", Args: []string{"daemon", "--host", "unix://"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, dto.ShutdownDTOFrom(tc.info))
		})
	}
}
