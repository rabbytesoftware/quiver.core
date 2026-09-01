package auth

import (
	"testing"
	"time"
)

func TestPairingCode_CanClaim_Cases(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name string
		code PairingCode
		want bool
	}{
		{
			name: "PendingAndNotExpired",
			code: PairingCode{State: PairingCodeStatePending, ExpiresAt: now.Add(time.Minute)},
			want: true,
		},
		{
			name: "PendingAndExpired",
			code: PairingCode{State: PairingCodeStatePending, ExpiresAt: now.Add(-time.Minute)},
			want: false,
		},
		{
			name: "PendingAndExpiresExactlyNow",
			code: PairingCode{State: PairingCodeStatePending, ExpiresAt: now},
			want: false,
		},
		{
			name: "AlreadyClaimed",
			code: PairingCode{State: PairingCodeStateClaimed, ExpiresAt: now.Add(time.Minute)},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.code.CanClaim(now); got != tc.want {
				t.Errorf("CanClaim() = %v, want %v", got, tc.want)
			}
		})
	}
}
