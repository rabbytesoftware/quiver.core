package domain

import "testing"

func TestRequirement_IsValid(t *testing.T) {
	testCases := []struct {
		name        string
		requirement Requirement
		expected    bool
	}{
		{
			name: "valid requirement",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   30,
				OS:       []OS{OSLinuxAMD64},
			},
			expected: true,
		},
		{
			name: "valid requirement with minimum values",
			requirement: Requirement{
				CpuCores: 1,
				MemoryGB: 1,
				DiskGB:   1,
				OS:       []OS{OSWindowsAMD64},
			},
			expected: true,
		},
		{
			name: "valid requirement — multiple OS",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   10,
				OS:       []OS{OSLinuxAMD64, OSWindowsAMD64},
			},
			expected: true,
		},
		{
			name: "valid requirement — no OS",
			requirement: Requirement{
				CpuCores: 1,
				MemoryGB: 1,
				DiskGB:   1,
			},
			expected: true,
		},
		{
			name: "invalid — zero cpu cores",
			requirement: Requirement{
				CpuCores: 0,
				MemoryGB: 4,
				DiskGB:   10,
				OS:       []OS{OSLinuxAMD64},
			},
			expected: false,
		},
		{
			name: "invalid — negative cpu cores",
			requirement: Requirement{
				CpuCores: -1,
				MemoryGB: 4,
				DiskGB:   10,
				OS:       []OS{OSLinuxAMD64},
			},
			expected: false,
		},
		{
			name: "invalid — zero memory",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 0,
				DiskGB:   10,
				OS:       []OS{OSLinuxAMD64},
			},
			expected: false,
		},
		{
			name: "invalid — zero disk",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   0,
				OS:       []OS{OSLinuxAMD64},
			},
			expected: false,
		},
		{
			name: "invalid — bad OS value",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   10,
				OS:       []OS{OS("invalid/os")},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.requirement.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() = %v for %+v, got %v", tc.expected, tc.requirement, result)
			}
		})
	}
}

func TestRequirement_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		requirement Requirement
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid requirement",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   30,
				OS:       []OS{OSLinuxAMD64},
			},
			expectError: false,
		},
		{
			name: "cpu cores below minimum",
			requirement: Requirement{
				CpuCores: 0,
				MemoryGB: 4,
				DiskGB:   10,
			},
			expectError: true,
			errorMsg:    "cpu_cores must be >= 1",
		},
		{
			name: "memory below minimum",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 0,
				DiskGB:   10,
			},
			expectError: true,
			errorMsg:    "memory_gb must be >= 1",
		},
		{
			name: "disk below minimum",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   0,
			},
			expectError: true,
			errorMsg:    "disk_gb must be >= 1",
		},
		{
			name: "invalid OS",
			requirement: Requirement{
				CpuCores: 2,
				MemoryGB: 4,
				DiskGB:   10,
				OS:       []OS{OS("invalid/os")},
			},
			expectError: true,
			errorMsg:    "invalid OS",
		},
		{
			name: "minimum valid values",
			requirement: Requirement{
				CpuCores: MinCPUCores,
				MemoryGB: MinMemoryGB,
				DiskGB:   MinDiskGB,
				OS:       []OS{OSLinuxAMD64},
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.requirement.Validate()
			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tc.errorMsg != "" && !contains(err.Error(), tc.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
			}
		})
	}
}

func TestRequirement_StructFields(t *testing.T) {
	requirement := Requirement{
		CpuCores: 4,
		MemoryGB: 8,
		DiskGB:   100,
		OS:       []OS{OSDarwinARM64, OSLinuxAMD64},
	}

	if requirement.CpuCores != 4 {
		t.Errorf("Expected CpuCores 4, got %d", requirement.CpuCores)
	}
	if requirement.MemoryGB != 8 {
		t.Errorf("Expected MemoryGB 8, got %d", requirement.MemoryGB)
	}
	if requirement.DiskGB != 100 {
		t.Errorf("Expected DiskGB 100, got %d", requirement.DiskGB)
	}
	if len(requirement.OS) != 2 {
		t.Errorf("Expected 2 OS values, got %d", len(requirement.OS))
	}
	if requirement.OS[0] != OSDarwinARM64 {
		t.Errorf("Expected first OS %q, got %q", OSDarwinARM64, requirement.OS[0])
	}
}

func TestRequirement_AllValidOS(t *testing.T) {
	validOS := []OS{
		OSLinuxAMD64,
		OSLinuxARM64,
		OSWindowsAMD64,
		OSWindowsARM64,
		OSDarwinAMD64,
		OSDarwinARM64,
	}

	for _, o := range validOS {
		r := Requirement{CpuCores: 1, MemoryGB: 1, DiskGB: 1, OS: []OS{o}}
		if !r.IsValid() {
			t.Errorf("Expected requirement with OS %q to be valid", o)
		}
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
