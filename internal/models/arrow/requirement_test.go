package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

func TestRequirement_IsValid(t *testing.T) {
	testCases := []struct {
		name        string
		requirement Requirement
		expected    bool
	}{
		{
			name: "valid requirement",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: true,
		},
		{
			name: "valid requirement with minimum values",
			requirement: Requirement{
				CpuCores:    1,
				Memory:      1,
				Disk:        1,
				NetworkMbps: 1,
				OS:          shared.OSWindowsAMD64,
			},
			expected: true,
		},
		{
			name: "invalid requirement - zero CPU cores",
			requirement: Requirement{
				CpuCores:    0,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - negative CPU cores",
			requirement: Requirement{
				CpuCores:    -1,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - zero memory",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      0,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - negative memory",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      -1,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - zero disk",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        0,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - negative disk",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        -1,
				NetworkMbps: 100,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - zero network",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 0,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - negative network",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: -1,
				OS:          shared.OSLinuxAMD64,
			},
			expected: false,
		},
		{
			name: "invalid requirement - invalid OS",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OS("invalid"),
			},
			expected: false,
		},
		{
			name: "invalid requirement - empty OS",
			requirement: Requirement{
				CpuCores:    2,
				Memory:      4096,
				Disk:        10240,
				NetworkMbps: 100,
				OS:          shared.OS(""),
			},
			expected: false,
		},
		{
			name: "invalid requirement - all zeros",
			requirement: Requirement{
				CpuCores: 0,
				Memory:   0,
				Disk:     0,
				OS:       shared.OS(""),
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.requirement.IsValid()
			if result != tc.expected {
				t.Errorf("Expected IsValid() to return %v for requirement %+v, got %v", tc.expected, tc.requirement, result)
			}
		})
	}
}

func TestRequirement_StructFields(t *testing.T) {
	// Test that all fields can be set and retrieved
	requirement := Requirement{
		CpuCores:    4,
		Memory:      8192,
		Disk:        20480,
		NetworkMbps: 1000,
		OS:          shared.OSDarwinARM64,
	}

	if requirement.CpuCores != 4 {
		t.Errorf("Expected CpuCores to be 4, got %d", requirement.CpuCores)
	}

	if requirement.Memory != 8192 {
		t.Errorf("Expected Memory to be 8192, got %d", requirement.Memory)
	}

	if requirement.Disk != 20480 {
		t.Errorf("Expected Disk to be 20480, got %d", requirement.Disk)
	}

	if requirement.NetworkMbps != 1000 {
		t.Errorf("Expected NetworkMbps to be 1000, got %d", requirement.NetworkMbps)
	}

	if requirement.OS != shared.OSDarwinARM64 {
		t.Errorf("Expected OS to be %q, got %q", shared.OSDarwinARM64, requirement.OS)
	}
}

func TestRequirement_ZeroValue(t *testing.T) {
	// Test zero value of Requirement
	var requirement Requirement

	if requirement.CpuCores != 0 {
		t.Errorf("Expected zero value CpuCores to be 0, got %d", requirement.CpuCores)
	}

	if requirement.Memory != 0 {
		t.Errorf("Expected zero value Memory to be 0, got %d", requirement.Memory)
	}

	if requirement.Disk != 0 {
		t.Errorf("Expected zero value Disk to be 0, got %d", requirement.Disk)
	}

	if requirement.NetworkMbps != 0 {
		t.Errorf("Expected zero value NetworkMbps to be 0, got %d", requirement.NetworkMbps)
	}

	if requirement.OS != "" {
		t.Errorf("Expected zero value OS to be empty, got %q", requirement.OS)
	}

	// Zero value requirement should be invalid
	if requirement.IsValid() {
		t.Error("Expected zero value requirement to be invalid")
	}
}

func TestRequirement_AllValidOS(t *testing.T) {
	// Test with all valid OS types
	validOSTypes := []shared.OS{
		shared.OSLinuxAMD64,
		shared.OSLinuxARM64,
		shared.OSWindowsAMD64,
		shared.OSWindowsARM64,
		shared.OSDarwinAMD64,
		shared.OSDarwinARM64,
	}

	for _, osType := range validOSTypes {
		requirement := Requirement{
			CpuCores:    2,
			Memory:      4096,
			Disk:        10240,
			NetworkMbps: 100,
			OS:          osType,
		}

		if !requirement.IsValid() {
			t.Errorf("Expected requirement with OS %q to be valid", osType)
		}
	}
}

func TestRequirement_EdgeCases(t *testing.T) {
	// Test edge cases with very large values
	requirement := Requirement{
		CpuCores:    1000000,
		Memory:      999999999,
		Disk:        999999999,
		NetworkMbps: 10000,
		OS:          shared.OSLinuxAMD64,
	}

	if !requirement.IsValid() {
		t.Error("Expected requirement with very large values to be valid")
	}

	// Test edge case with minimum valid values
	requirement = Requirement{
		CpuCores:    1,
		Memory:      1,
		Disk:        1,
		NetworkMbps: 1,
		OS:          shared.OSLinuxAMD64,
	}

	if !requirement.IsValid() {
		t.Error("Expected requirement with minimum valid values to be valid")
	}
}

func TestRequirement_PartialInvalidity(t *testing.T) {
	// Test that if any field is invalid, the whole requirement is invalid
	baseRequirement := Requirement{
		CpuCores:    2,
		Memory:      4096,
		Disk:        10240,
		NetworkMbps: 100,
		OS:          shared.OSLinuxAMD64,
	}

	// Test each field being invalid while others are valid
	testCases := []struct {
		name        string
		requirement Requirement
	}{
		{
			name: "invalid CPU only",
			requirement: Requirement{
				CpuCores:    0,
				Memory:      baseRequirement.Memory,
				Disk:        baseRequirement.Disk,
				NetworkMbps: baseRequirement.NetworkMbps,
				OS:          baseRequirement.OS,
			},
		},
		{
			name: "invalid Memory only",
			requirement: Requirement{
				CpuCores:    baseRequirement.CpuCores,
				Memory:      0,
				Disk:        baseRequirement.Disk,
				NetworkMbps: baseRequirement.NetworkMbps,
				OS:          baseRequirement.OS,
			},
		},
		{
			name: "invalid Disk only",
			requirement: Requirement{
				CpuCores:    baseRequirement.CpuCores,
				Memory:      baseRequirement.Memory,
				Disk:        0,
				NetworkMbps: baseRequirement.NetworkMbps,
				OS:          baseRequirement.OS,
			},
		},
		{
			name: "invalid NetworkMbps only",
			requirement: Requirement{
				CpuCores:    baseRequirement.CpuCores,
				Memory:      baseRequirement.Memory,
				Disk:        baseRequirement.Disk,
				NetworkMbps: 0,
				OS:          baseRequirement.OS,
			},
		},
		{
			name: "invalid OS only",
			requirement: Requirement{
				CpuCores:    baseRequirement.CpuCores,
				Memory:      baseRequirement.Memory,
				Disk:        baseRequirement.Disk,
				NetworkMbps: baseRequirement.NetworkMbps,
				OS:          shared.OS("invalid"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.requirement.IsValid() {
				t.Errorf("Expected requirement to be invalid when %s", tc.name)
			}
		})
	}
}
