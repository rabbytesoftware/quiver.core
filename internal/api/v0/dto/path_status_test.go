package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

func TestPathStatusDTOFrom(t *testing.T) {
	testCases := []struct {
		name   string
		status models.PathStatus
		want   dto.PathStatusDTO
	}{
		{
			name:   "nil files becomes empty slice",
			status: models.PathStatus{BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true},
			want: dto.PathStatusDTO{
				BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{},
			},
		},
		{
			name: "files are preserved",
			status: models.PathStatus{
				BinDir: "/home/u/.quiver/bin", OnPath: false, Configured: false,
				Files: []string{"/home/u/.zshrc"},
			},
			want: dto.PathStatusDTO{
				BinDir: "/home/u/.quiver/bin", OnPath: false, Configured: false,
				Files: []string{"/home/u/.zshrc"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := dto.PathStatusDTOFrom(tc.status)
			assert.Equal(t, tc.want, got)
		})
	}
}
