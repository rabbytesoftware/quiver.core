package auth

import "testing"

func TestDevice_IsActive_Cases(t *testing.T) {
	testCases := []struct {
		name  string
		state DeviceState
		want  bool
	}{
		{name: "Active", state: DeviceStateActive, want: true},
		{name: "Revoked", state: DeviceStateRevoked, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := Device{State: tc.state}
			if got := d.IsActive(); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}
