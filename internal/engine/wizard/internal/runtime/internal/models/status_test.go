package models

import "testing"

func TestStatus_String(t *testing.T) {
	cases := []struct {
		status Status
		want   string
	}{
		{StatusPrepared, "prepared"},
		{StatusRunning, "running"},
		{StatusStopping, "stopping"},
		{StatusKilling, "killing"},
		{StatusFinished, "finished"},
	}
	for _, c := range cases {
		if got := c.status.String(); got != c.want {
			t.Errorf("Status(%q).String() = %q, want %q", c.status, got, c.want)
		}
	}
}

func TestStatus_IsActive(t *testing.T) {
	active := []Status{StatusRunning, StatusStopping, StatusKilling}
	for _, s := range active {
		if !s.IsActive() {
			t.Errorf("IsActive() = false for %q, want true", s)
		}
	}

	inactive := []Status{StatusPrepared, StatusFinished}
	for _, s := range inactive {
		if s.IsActive() {
			t.Errorf("IsActive() = true for %q, want false", s)
		}
	}
}

func TestStatus_IsFinished(t *testing.T) {
	if !StatusFinished.IsFinished() {
		t.Error("IsFinished() = false for StatusFinished, want true")
	}

	others := []Status{StatusPrepared, StatusRunning, StatusStopping, StatusKilling}
	for _, s := range others {
		if s.IsFinished() {
			t.Errorf("IsFinished() = true for %q, want false", s)
		}
	}
}
