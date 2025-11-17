package requirements

import (
	"context"
	"runtime"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/requirement"
	"github.com/rabbytesoftware/quiver/internal/models/system"
)

func TestNewRequirements(t *testing.T) {
	req := NewRequirements()
	if req == nil {
		t.Fatal("NewRequirements() returned nil")
	}
}

func TestRequirements_InterfaceCompliance(t *testing.T) {
	var _ SRVInterface = &Requirements{}
}

func TestRequirements_Validate(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	currentOS := getCurrentSystemOS()

	tests := []struct {
		name        string
		requirement *requirement.Requirement
		wantValid   bool
		wantErr     bool
	}{
		{
			name:        "nil requirement",
			requirement: nil,
			wantValid:   false,
			wantErr:     true,
		},
		{
			name: "valid requirement for current system",
			requirement: &requirement.Requirement{
				CpuCores: 1,
				Memory:   100,
				Disk:     100,
				OS:       currentOS,
			},
			wantValid: true,
			wantErr:   false,
		},
		{
			name: "invalid OS requirement",
			requirement: &requirement.Requirement{
				CpuCores: 1,
				Memory:   100,
				Disk:     100,
				OS:       "invalid/arch",
			},
			wantValid: false,
			wantErr:   true,
		},
		{
			name: "excessive CPU requirement",
			requirement: &requirement.Requirement{
				CpuCores: 99999,
				Memory:   100,
				Disk:     100,
				OS:       currentOS,
			},
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.Validate(ctx, tt.requirement)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ValidateOS(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	currentOS := getCurrentSystemOS()

	tests := []struct {
		name      string
		os        system.OS
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "valid current OS",
			os:        currentOS,
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "invalid OS format",
			os:        "invalid",
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "wrong OS",
			os:        getWrongOS(),
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.ValidateOS(ctx, tt.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateOS() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ValidateArch(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	tests := []struct {
		name      string
		arch      string
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "valid current architecture",
			arch:      runtime.GOARCH,
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "invalid architecture",
			arch:      "invalid_arch",
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.ValidateArch(ctx, tt.arch)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateArch() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ValidateCPU(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	tests := []struct {
		name      string
		cpuCores  int
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "low CPU requirement",
			cpuCores:  1,
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "invalid CPU requirement (zero)",
			cpuCores:  0,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "invalid CPU requirement (negative)",
			cpuCores:  -1,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "excessive CPU requirement",
			cpuCores:  99999,
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.ValidateCPU(ctx, tt.cpuCores)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCPU() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateCPU() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ValidateMemory(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	tests := []struct {
		name      string
		memory    int
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "low memory requirement",
			memory:    100, // 100 MB
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "invalid memory requirement (zero)",
			memory:    0,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "invalid memory requirement (negative)",
			memory:    -1,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "excessive memory requirement",
			memory:    999999999, // 999GB
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.ValidateMemory(ctx, tt.memory)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMemory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateMemory() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ValidateDisk(t *testing.T) {
	req := NewRequirements()
	ctx := context.Background()

	tests := []struct {
		name      string
		disk      int
		wantValid bool
		wantErr   bool
	}{
		{
			name:      "low disk requirement",
			disk:      100, // 100 MB
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "invalid disk requirement (zero)",
			disk:      0,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "invalid disk requirement (negative)",
			disk:      -1,
			wantValid: false,
			wantErr:   true,
		},
		{
			name:      "excessive disk requirement",
			disk:      999999999, // 999GB
			wantValid: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := req.ValidateDisk(ctx, tt.disk)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDisk() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if valid != tt.wantValid {
				t.Errorf("ValidateDisk() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}

func TestRequirements_ContextCancellation(t *testing.T) {
	req := NewRequirements()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	currentOS := getCurrentSystemOS()

	// Test all methods with cancelled context
	t.Run("Validate", func(t *testing.T) {
		_, err := req.Validate(ctx, &requirement.Requirement{
			CpuCores: 1,
			Memory:   100,
			Disk:     100,
			OS:       currentOS,
		})
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("ValidateOS", func(t *testing.T) {
		_, err := req.ValidateOS(ctx, currentOS)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("ValidateArch", func(t *testing.T) {
		_, err := req.ValidateArch(ctx, runtime.GOARCH)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("ValidateCPU", func(t *testing.T) {
		_, err := req.ValidateCPU(ctx, 1)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("ValidateMemory", func(t *testing.T) {
		_, err := req.ValidateMemory(ctx, 100)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})

	t.Run("ValidateDisk", func(t *testing.T) {
		_, err := req.ValidateDisk(ctx, 100)
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})
}

func getCurrentSystemOS() system.OS {
	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "amd64" {
			return system.OSLinuxAMD64
		}
		return system.OSLinuxARM64
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return system.OSDarwinAMD64
		}
		return system.OSDarwinARM64
	case "windows":
		if runtime.GOARCH == "amd64" {
			return system.OSWindowsAMD64
		}
		return system.OSWindowsARM64
	default:
		return system.OSLinuxAMD64
	}
}

func getWrongOS() system.OS {
	current := runtime.GOOS
	switch current {
	case "linux":
		return system.OSWindowsAMD64
	case "windows":
		return system.OSLinuxAMD64
	case "darwin":
		return system.OSLinuxAMD64
	default:
		return system.OSWindowsAMD64
	}
}
