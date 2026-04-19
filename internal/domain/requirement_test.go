package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequirement_IsValid(t *testing.T) {
	tests := []struct {
		name string
		r    Requirement
		want bool
	}{
		{
			name: "valid — minimum values",
			r:    Requirement{CpuCores: 1, MemoryGB: 1, DiskGB: 1},
			want: true,
		},
		{
			name: "valid — large values",
			r:    Requirement{CpuCores: 16, MemoryGB: 64, DiskGB: 500},
			want: true,
		},
		{
			name: "invalid — zero cpu",
			r:    Requirement{CpuCores: 0, MemoryGB: 4, DiskGB: 10},
			want: false,
		},
		{
			name: "invalid — negative cpu",
			r:    Requirement{CpuCores: -1, MemoryGB: 4, DiskGB: 10},
			want: false,
		},
		{
			name: "invalid — zero memory",
			r:    Requirement{CpuCores: 2, MemoryGB: 0, DiskGB: 10},
			want: false,
		},
		{
			name: "invalid — zero disk",
			r:    Requirement{CpuCores: 2, MemoryGB: 4, DiskGB: 0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.r.IsValid())
		})
	}
}

func TestRequirement_Validate(t *testing.T) {
	tests := []struct {
		name    string
		r       Requirement
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid",
			r:    Requirement{CpuCores: 2, MemoryGB: 4, DiskGB: 30},
		},
		{
			name:    "cpu below minimum",
			r:       Requirement{CpuCores: 0, MemoryGB: 4, DiskGB: 10},
			wantErr: true,
			errMsg:  "cpu_cores must be >= 1",
		},
		{
			name:    "memory below minimum",
			r:       Requirement{CpuCores: 2, MemoryGB: 0, DiskGB: 10},
			wantErr: true,
			errMsg:  "memory_gb must be >= 1",
		},
		{
			name:    "disk below minimum",
			r:       Requirement{CpuCores: 2, MemoryGB: 4, DiskGB: 0},
			wantErr: true,
			errMsg:  "disk_gb must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
